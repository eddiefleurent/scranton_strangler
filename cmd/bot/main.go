// Package main provides the entry point for the SPY short strangle trading bot.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/orders"
	"github.com/eddiefleurent/scranton_strangler/internal/retry"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
	"github.com/eddiefleurent/scranton_strangler/internal/util"
	"github.com/google/uuid"
)

// Bot represents the main trading bot instance.
type Bot struct {
	config        *config.Config
	broker        broker.Broker
	strategy      *strategy.StrangleStrategy
	storage       storage.Interface
	logger        *log.Logger
	stop          chan struct{}
	ctx           context.Context // Main bot context for operations
	orderManager  *orders.Manager
	retryClient   *retry.Client
	nyLocation    *time.Location // Cached NY timezone location
	lastPnLUpdate time.Time      // Last time P&L was persisted to reduce write amplification
	pnlThrottle   time.Duration  // Minimum interval between P&L updates
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create logger
	logger := log.New(os.Stdout, "[BOT] ", log.LstdFlags|log.Lshortfile)

	logger.Printf("Starting SPY Strangle Bot in %s mode", cfg.Environment.Mode)
	if cfg.IsPaperTrading() {
		logger.Println("🏳️ PAPER TRADING MODE - No real money at risk")
	} else {
		logger.Println("💰 LIVE TRADING MODE - Real money at risk!")
		logger.Println("Waiting 10 seconds to confirm...")
		time.Sleep(10 * time.Second)
	}

	// Initialize bot
	bot := &Bot{
		config:        cfg,
		logger:        logger,
		stop:          make(chan struct{}),
		pnlThrottle:   30 * time.Second,           // Throttle P&L updates to every 30 seconds minimum
		lastPnLUpdate: time.Now().Add(-time.Hour), // Initialize to past time to allow immediate first update
	}

	// Cache NY timezone location
	if loc, err := time.LoadLocation("America/New_York"); err != nil {
		log.Fatalf("Failed to load NY timezone: %v", err)
	} else {
		bot.nyLocation = loc
	}

	// Initialize broker client
	tradierClient, err := broker.NewTradierClient(
		cfg.Broker.APIKey,
		cfg.Broker.AccountID,
		cfg.IsPaperTrading(),
		cfg.Broker.UseOTOCO,
		cfg.Strategy.Exit.ProfitTarget,
	)
	if err != nil {
		log.Fatalf("Failed to create Tradier client: %v", err)
	}

	// Wrap with circuit breaker for resilience
	bot.broker = broker.NewCircuitBreakerBroker(tradierClient)

	// Initialize storage
	storagePath := cfg.Storage.Path
	store, err := storage.NewStorage(storagePath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	bot.storage = store

	// Initialize strategy
	strategyConfig := &strategy.Config{
		Symbol:              cfg.Strategy.Symbol,
		DTETarget:           cfg.Strategy.Entry.TargetDTE,
		DeltaTarget:         cfg.Strategy.Entry.Delta / 100, // Convert from percentage
		ProfitTarget:        cfg.Strategy.Exit.ProfitTarget,
		MaxDTE:              cfg.Strategy.Exit.MaxDTE,
		AllocationPct:       cfg.Strategy.AllocationPct,
		MinIVR:              cfg.Strategy.Entry.MinIVR,
		MinCredit:           cfg.Strategy.Entry.MinCredit,
		EscalateLossPct:     cfg.Strategy.EscalateLossPct,
		StopLossPct:         cfg.Strategy.Exit.StopLossPct,
		MaxPositionLoss:     cfg.Risk.MaxPositionLoss,
		MaxContracts:        cfg.Risk.MaxContracts,
		UseMockHistoricalIV: cfg.Strategy.UseMockHistoricalIV,
		FailOpenOnIVError:   cfg.Strategy.FailOpenOnIVError, // Configurable fail-open behavior
	}
	bot.strategy = strategy.NewStrangleStrategy(bot.broker, strategyConfig, logger, bot.storage)

	// Initialize order manager
	bot.orderManager = orders.NewManager(bot.broker, bot.storage, logger, bot.stop)

	// Initialize retry client
	bot.retryClient = retry.NewClient(bot.broker, logger)

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Println("Shutdown signal received, stopping bot...")
		close(bot.stop)
		cancel()
	}()

	// Run the bot
	if err := bot.Run(ctx); err != nil {
		logger.Fatalf("Bot error: %v", err)
	}

	logger.Println("Bot stopped successfully")
}

// Run starts the bot's main execution loop.
func (b *Bot) Run(ctx context.Context) error {
	b.ctx = ctx // Store context for use in operations
	b.logger.Println("Bot starting main loop...")

	// Verify broker connection with timeout
	b.logger.Println("Verifying broker connection...")
	balanceChan := make(chan float64, 1)
	errChan := make(chan error, 1)

	go func() {
		balance, err := b.broker.GetAccountBalance()
		if err != nil {
			errChan <- err
		} else {
			balanceChan <- balance
		}
	}()

	select {
	case balance := <-balanceChan:
		b.logger.Printf("Connected to broker. Account balance: $%.2f", balance)
	case err := <-errChan:
		return fmt.Errorf("failed to connect to broker: %w", err)
	case <-time.After(10 * time.Second):
		return fmt.Errorf("broker health check timed out after 10 seconds")
	case <-ctx.Done():
		return fmt.Errorf("broker health check cancelled: %w", ctx.Err())
	}

	// Main trading loop
	ticker := time.NewTicker(b.config.GetCheckInterval())
	defer ticker.Stop()

	// Run immediately on start
	b.runTradingCycle()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-b.stop:
			return nil
		case <-ticker.C:
			b.runTradingCycle()
		}
	}
}

func (b *Bot) runTradingCycle() {
	now := time.Now()
	if b.nyLocation != nil {
		now = now.In(b.nyLocation)
	} else {
		b.logger.Printf("Warning: NY timezone not cached, using system time")
	}

	// Check if within trading hours
	isWithinHours := b.config.IsWithinTradingHours(now)
	if !isWithinHours {
		if !b.config.Schedule.AfterHoursCheck {
			b.logger.Printf("Outside trading hours (%s - %s), skipping cycle",
				b.config.Schedule.TradingStart, b.config.Schedule.TradingEnd)
			return
		}
		b.logger.Printf("Outside trading hours (%s - %s), running after-hours check",
			b.config.Schedule.TradingStart, b.config.Schedule.TradingEnd)
	}

	b.logger.Println("Starting trading cycle...")

	// Check for existing position
	hasPosition := b.checkExistingPosition(now)

	if hasPosition {
		// Check exit conditions
		b.logger.Println("Position exists, checking exit conditions...")
		position := b.storage.GetCurrentPosition()
		shouldExit, reason := b.strategy.CheckExitConditions(position)
		if shouldExit {
			b.logger.Printf("Exit signal: %s", reason)
			b.executeExit(b.ctx, reason)
		} else {
			b.logger.Println("No exit conditions met, continuing to monitor")
		}

		// Check for adjustments (Phase 2) - only during regular hours
		if b.config.Strategy.Adjustments.Enabled && isWithinHours {
			b.checkAdjustments()
		}
	} else {
		// Check entry conditions - only during regular trading hours
		if isWithinHours {
			b.logger.Println("No position, checking entry conditions...")
			canEnter, reason := b.strategy.CheckEntryConditions()
			if canEnter {
				b.logger.Printf("Entry signal: %s", reason)
				b.executeEntry()
			} else {
				b.logger.Printf("Entry conditions not met: %s", reason)
			}
		} else {
			b.logger.Println("After-hours: Skipping entry checks, only monitoring existing positions")
		}
	}

	b.logger.Println("Trading cycle complete")
}

func (b *Bot) checkExistingPosition(now time.Time) bool {
	position := b.storage.GetCurrentPosition()
	if position == nil {
		return false
	}

	// Check if position is already closed
	if position.GetCurrentState() == models.StateClosed {
		// Position needs completion if it has exit order/reason but not in history
		needsCompletion := position.ExitOrderID != "" &&
			position.ExitReason != "" &&
			!b.storage.HasInHistory(position.ID)

		if needsCompletion {
			b.logger.Printf("Position %s was closed by exit order, completing position close", position.ID)
			exitReason := strategy.ExitReason(position.ExitReason)
			b.completePositionClose(position, exitReason)
		} else if !b.storage.HasInHistory(position.ID) {
			b.logger.Printf("Position %s is closed but missing completion data", position.ID)
		} else {
			b.logger.Printf("Position %s already finalized in history", position.ID)
		}
		return false
	}

	// Calculate real-time P&L
	realTimePnL, err := b.strategy.CalculatePositionPnL(position)
	if err != nil {
		b.logger.Printf("Warning: Could not calculate real-time P&L: %v", err)
		realTimePnL = position.CurrentPnL // Fall back to stored value
	} else {
		delta := math.Abs(realTimePnL - position.CurrentPnL)

		// Throttle P&L updates to reduce write amplification
		// Update if: significant change (>= $1.00) OR enough time has passed since last update
		shouldUpdate := delta >= 1.00 || now.Sub(b.lastPnLUpdate) >= b.pnlThrottle

		if shouldUpdate {
			position.CurrentPnL = realTimePnL
			if err := b.storage.SetCurrentPosition(position); err != nil {
				b.logger.Printf("Warning: Failed to update position P&L: %v", err)
			} else {
				b.lastPnLUpdate = now
				if err := b.storage.Save(); err != nil {
					b.logger.Printf("Warning: Failed to persist P&L update: %v", err)
				}
			}
		}
	}

	b.logger.Printf("Found existing position: %s %s Put %.0f / Call %.0f (DTE: %d, P&L: $%.2f)",
		position.Symbol, position.Expiration.Format("2006-01-02"),
		position.PutStrike, position.CallStrike,
		position.CalculateDTE(), realTimePnL)

	return true
}

func (b *Bot) executeEntry() {
	b.logger.Println("Executing entry...")

	// Find strikes
	order, err := b.strategy.FindStrangleStrikes()
	if err != nil {
		b.logger.Printf("Failed to find strikes: %v", err)
		return
	}

	b.logger.Printf("Found strangle: Put %.0f / Call %.0f, Credit: $%.2f",
		order.PutStrike, order.CallStrike, order.Credit)

	// Risk check
	if order.Quantity > b.config.Risk.MaxContracts {
		order.Quantity = b.config.Risk.MaxContracts
		b.logger.Printf("Position size limited to %d contracts", order.Quantity)
	}

	// Guard against non-positive sizes after risk limiting
	if order.Quantity <= 0 {
		b.logger.Printf("ERROR: Computed order size is non-positive (%d), aborting order placement", order.Quantity)
		return
	}

	// Parse expiration early to fail-fast before any live order is placed
	expirationTime, err := time.Parse("2006-01-02", order.Expiration)
	if err != nil {
		b.logger.Printf("Failed to parse expiration date %q: %v", order.Expiration, err)
		return
	}

	// Place order
	b.logger.Printf("Placing strangle order for %d contracts...", order.Quantity)

	// Get appropriate tick size for the symbol
	tickSize, err := b.broker.GetTickSize(order.Symbol)
	if err != nil {
		b.logger.Printf("Warning: Failed to get tick size for %s, using default 0.01: %v", order.Symbol, err)
		tickSize = 0.01
	}

	px := util.FloorToTick(order.Credit, tickSize)
	b.logger.Printf("Using tick size %.4f for symbol %s, rounded price: $%.2f", tickSize, order.Symbol, px)

	// Generate deterministic client-order ID for potential deduplication
	canonicalString := fmt.Sprintf("entry-%s-%s-%.2f-%.2f-%d",
		order.Symbol,
		order.Expiration,
		order.PutStrike,
		order.CallStrike,
		order.Quantity)

	hash := sha256.Sum256([]byte(canonicalString))
	clientOrderID := "entry-" + hex.EncodeToString(hash[:])[:8]

	placedOrder, err := b.broker.PlaceStrangleOrder(
		order.Symbol,
		order.PutStrike,
		order.CallStrike,
		order.Expiration,
		order.Quantity,
		px,            // limit price (rounded to tick)
		false,         // not preview
		"day",         // duration
		clientOrderID, // stable client-order ID for deduplication
	)

	if err != nil {
		b.logger.Printf("Failed to place order: %v", err)
		return
	}

	b.logger.Printf("Order placed successfully: %d", placedOrder.Order.ID)

	// Save position state
	positionID := generatePositionID()

	// Create new position
	position := models.NewPosition(
		positionID,
		order.Symbol,
		order.PutStrike,
		order.CallStrike,
		expirationTime,
		order.Quantity,
	)

	// Set entry details
	position.CreditReceived = order.Credit
	position.EntryLimitPrice = px
	position.EntrySpot = order.SpotPrice
	position.DTE = position.CalculateDTE()

	// Set real entry IVR
	position.EntryIVR = b.strategy.GetCurrentIVR()

	// Set the broker order ID
	position.EntryOrderID = fmt.Sprintf("%d", placedOrder.Order.ID)

	// Initialize position state to submitted
	if err := position.TransitionState(models.StateSubmitted, "order_placed"); err != nil {
		b.logger.Printf("Failed to set position state: %v", err)
		return
	}

	// Save position to storage
	if err := b.storage.SetCurrentPosition(position); err != nil {
		b.logger.Printf("Failed to save position: %v", err)
		return
	}
	if err := b.storage.Save(); err != nil {
		b.logger.Printf("Warning: Failed to persist position save: %v", err)
	}

	b.logger.Printf("Position saved: ID=%s, LimitPrice=$%.2f, DTE=%d",
		position.ID, position.EntryLimitPrice, position.DTE)

	// Start order status polling in background
	go b.orderManager.PollOrderStatus(position.ID, placedOrder.Order.ID, true)
}

func (b *Bot) executeExit(ctx context.Context, reason strategy.ExitReason) {
	b.logger.Printf("Executing exit: %s", reason)

	position := b.storage.GetCurrentPosition()
	if position == nil {
		b.logger.Printf("No position to exit")
		return
	}

	if !b.isPositionReadyForExit(position) {
		return
	}

	b.logPositionClose(position)

	maxDebit := b.calculateMaxDebit(position, reason)

	if maxDebit <= 0 {
		b.logger.Printf("Skipping close order for position %s: calculated maxDebit $%.2f is invalid (must be > 0)", position.ID, maxDebit)
		return
	}

	// Get appropriate tick size for the symbol and round maxDebit up to tick size
	tickSize, err := b.broker.GetTickSize(position.Symbol)
	if err != nil {
		b.logger.Printf("Warning: Failed to get tick size for %s, using default 0.01: %v", position.Symbol, err)
		tickSize = 0.01
	}

	roundedMaxDebit := util.CeilToTick(maxDebit, tickSize)
	b.logger.Printf("Using tick size %.4f for symbol %s, rounded max debit: $%.2f (was $%.2f)", tickSize, position.Symbol, roundedMaxDebit, maxDebit)
	closeOrder, err := b.retryClient.ClosePositionWithRetry(ctx, position, roundedMaxDebit)
	if err != nil {
		b.logger.Printf("Failed to place close order after retries: %v", err)
		return
	}

	b.logger.Printf("Close order placed successfully: %d", closeOrder.Order.ID)

	// Store close order metadata and set position to adjusting state
	position.ExitOrderID = fmt.Sprintf("%d", closeOrder.Order.ID)
	position.ExitReason = string(reason)
	if err := position.TransitionState(models.StateAdjusting, "close_order_placed"); err != nil {
		b.logger.Printf("Warning: failed to transition to adjusting state: %v", err)
	}

	if err := b.storage.SetCurrentPosition(position); err != nil {
		b.logger.Printf("Failed to save position with close order ID: %v", err)
		return
	}
	if err := b.storage.Save(); err != nil {
		b.logger.Printf("Warning: Failed to persist position update: %v", err)
	}

	b.logger.Printf("Position %s transitioned to adjusting state, monitoring close order %d", position.ID, closeOrder.Order.ID)

	// Start order status polling for close order in background
	go b.orderManager.PollOrderStatus(position.ID, closeOrder.Order.ID, false)
}

func (b *Bot) isPositionReadyForExit(position *models.Position) bool {
	currentState := position.GetCurrentState()
	if currentState == models.StateClosed {
		b.logger.Printf("Position %s is already closed, skipping duplicate close attempt", position.ID)
		return false
	}

	// Helper function to check if state is a management state
	isManagementState := func(state models.PositionState) bool {
		return state == models.StateFirstDown ||
			state == models.StateSecondDown ||
			state == models.StateThirdDown ||
			state == models.StateFourthDown
	}

	// Always allow exits from Open state or management states
	if currentState == models.StateOpen || isManagementState(currentState) {
		return true
	}

	// Allow controlled re-attempts from Adjusting state if there is no active ExitOrderID
	// or the prior close order has terminal status (allowing idempotent retries)
	if currentState == models.StateAdjusting {
		// Check if there's no active exit order ID (order was never placed or was cleared)
		if position.ExitOrderID == "" {
			b.logger.Printf("Position %s in Adjusting state with no active exit order, allowing re-attempt", position.ID)
			return true
		}

		// TODO: Add order status check here to allow re-attempts if prior order is terminal (cancelled, rejected, filled)
		// For now, block Adjusting state exits to prevent duplicate orders
		b.logger.Printf("Position %s in Adjusting state with active exit order %s, blocking duplicate close attempt",
			position.ID, position.ExitOrderID)
		return false
	}

	// For all other states (Submitted, Idle, Error, Rolling)
	b.logger.Printf("Position %s is in state %s, not eligible for close (only Open, management, or controlled Adjusting states allowed)",
		position.ID, currentState)
	return false
}

func (b *Bot) logPositionClose(position *models.Position) {
	b.logger.Printf("Closing position: %s %s Put %.0f / Call %.0f (State: %s)",
		position.Symbol, position.Expiration.Format("2006-01-02"),
		position.PutStrike, position.CallStrike, position.GetCurrentState())
}

func (b *Bot) calculateMaxDebit(position *models.Position, reason strategy.ExitReason) float64 {
	currentVal, cvErr := b.strategy.GetCurrentPositionValue(position)

	// Get net credit including adjustments (can be negative for debit-heavy positions)
	netCredit := position.GetNetCredit()
	absNetCredit := math.Abs(netCredit)

	// Guardrails for config - prevent invalid calculations that could block exits
	pt := b.config.Strategy.Exit.ProfitTarget
	if pt < 0 || pt > 1 {
		b.logger.Printf("ERROR: Invalid ProfitTarget %.3f, must be in [0,1]. Using default 0.50", pt)
		pt = 0.50
	}

	sl := b.config.Strategy.Exit.StopLossPct
	if sl <= 1.0 {
		b.logger.Printf("ERROR: Invalid StopLossPct %.3f, must be > 1.0 (multiple of credit). Using default 2.5", sl)
		sl = 2.5
	}

	switch reason {
	case strategy.ExitReasonProfitTarget:
		// Close at max debit that locks in the configured profit fraction:
		// debit = |netCredit| * (1 - pt). Example: netCredit=$2, pt=0.5 → debit=$1.
		result := absNetCredit * (1.0 - pt)
		if result <= 0 {
			b.logger.Printf("ERROR: Calculated profit target debit (%.2f) is invalid. NetCredit: %.2f, AbsNetCredit: %.2f, PT: %.2f",
				result, netCredit, absNetCredit, pt)
			// Emergency fallback: allow any debit > 0 to prevent position lock-in
			result = absNetCredit * 0.01
		}
		return result
	case strategy.ExitReasonTime:
		if cvErr == nil && position.Quantity != 0 {
			return currentVal / (float64(position.Quantity) * 100)
		}
		// Fallback: use profit target when quantity is zero or value lookup failed
		result := absNetCredit * (1.0 - pt)
		if result <= 0 {
			b.logger.Printf("ERROR: Fallback profit target debit (%.2f) is invalid. Using emergency value", result)
			result = absNetCredit * 0.01
		}
		return result
	case strategy.ExitReasonStopLoss:
		if cvErr == nil && position.Quantity != 0 {
			return currentVal / (float64(position.Quantity) * 100)
		}
		// Fallback: treat StopLossPct as a debit multiple of |netCredit| (e.g., 2.5x)
		result := absNetCredit * sl
		if result <= 0 {
			b.logger.Printf("ERROR: Calculated stop loss debit (%.2f) is invalid. NetCredit: %.2f, AbsNetCredit: %.2f, SL: %.2f",
				result, netCredit, absNetCredit, sl)
			// Emergency fallback: allow reasonable stop loss
			result = absNetCredit * 2.0
		}
		return result
	default:
		return absNetCredit * 1.0
	}
}

func (b *Bot) completePositionClose(
	position *models.Position,
	reason strategy.ExitReason,
) {
	actualPnL := b.calculateActualPnL(position, reason)
	b.logPnL(position, actualPnL)

	if err := b.storage.ClosePosition(actualPnL, string(reason)); err != nil {
		b.logger.Printf("Failed to close position in storage: %v", err)
		return
	}

	b.logger.Printf("Position closed successfully: %s", reason)

	stats := b.storage.GetStatistics()
	b.logger.Printf("Trade Statistics - Total: %d, Win Rate: %.1f%%, Total P&L: $%.2f",
		stats.TotalTrades, stats.WinRate, stats.TotalPnL)
}

func (b *Bot) calculateActualPnL(position *models.Position, reason strategy.ExitReason) float64 {
	actualPnL, err := b.strategy.CalculatePositionPnL(position)
	if err != nil {
		b.logger.Printf("Warning: Could not calculate real P&L, using estimated value: %v", err)
		if reason == strategy.ExitReasonProfitTarget {
			pt := b.config.Strategy.Exit.ProfitTarget
			if pt < 0 || pt > 1 {
				pt = 0.50
			}
			netCredit := position.GetNetCredit()
			return math.Abs(netCredit) * float64(position.Quantity) * 100 * pt
		}
		multiple := b.config.Strategy.Exit.StopLossPct
		if multiple <= 1.0 {
			multiple = 2.5
		}
		netCredit := position.GetNetCredit()
		return -1 * math.Abs(netCredit) * multiple * float64(position.Quantity) * 100
	}
	return actualPnL
}

func (b *Bot) logPnL(position *models.Position, actualPnL float64) {
	netCredit := position.GetNetCredit()
	denom := math.Abs(netCredit) * float64(position.Quantity) * 100
	if denom <= 0 {
		b.logger.Printf("Position P&L: $%.2f (net credit unknown)", actualPnL)
		return
	}
	percent := (actualPnL / denom) * 100
	b.logger.Printf("Position P&L: $%.2f (%.1f%% of total net credit)", actualPnL, percent)
}

func (b *Bot) checkAdjustments() {
	b.logger.Println("Checking for adjustments...")
	// TODO: Implement adjustment logic (Phase 2)
}

// generatePositionID creates a unique ID for positions using UUID for guaranteed uniqueness
func generatePositionID() string {
	// Use UUID for guaranteed uniqueness
	return uuid.New().String()
}
