// Package main provides the entry point for the SPY short strangle trading bot.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/orders"
	"github.com/eddiefleurent/scranton_strangler/internal/retry"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
)

// Bot represents the main trading bot instance.
type Bot struct {
	config       *config.Config
	broker       broker.Broker
	strategy     *strategy.StrangleStrategy
	storage      storage.Interface
	logger       *log.Logger
	stop         chan struct{}
	ctx          context.Context // Main bot context for operations
	orderManager *orders.Manager
	retryClient  *retry.Client
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
		config: cfg,
		logger: logger,
		stop:   make(chan struct{}),
	}

	// Initialize broker client
	tradierClient := broker.NewTradierClient(
		cfg.Broker.APIKey,
		cfg.Broker.AccountID,
		cfg.IsPaperTrading(),
		cfg.Broker.UseOTOCO,
		cfg.Strategy.Exit.ProfitTarget,
	)

	// Wrap with circuit breaker for resilience
	bot.broker = broker.NewCircuitBreakerBroker(tradierClient)

	// Initialize storage
	storagePath := cfg.Storage.Path
	if storagePath == "" {
		storagePath = "positions.json"
	}
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
		UseMockHistoricalIV: cfg.Strategy.UseMockHistoricalIV,
	}
	bot.strategy = strategy.NewStrangleStrategy(bot.broker, strategyConfig)

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

	// Verify broker connection
	b.logger.Println("Verifying broker connection...")
	balance, err := b.broker.GetAccountBalance()
	if err != nil {
		return fmt.Errorf("failed to connect to broker: %w", err)
	}
	b.logger.Printf("Connected to broker. Account balance: $%.2f", balance)

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
	hasPosition := b.checkExistingPosition()

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

func (b *Bot) checkExistingPosition() bool {
	position := b.storage.GetCurrentPosition()
	if position == nil {
		return false
	}

	// Check if position is already closed
	if position.GetCurrentState() == models.StateClosed {
		// Check if this position was just closed by an exit order and needs completion
		if position.ExitOrderID != "" && position.ExitReason != "" {
			// Prevent double-finalization if already recorded in history
			for _, h := range b.storage.GetHistory() {
				if h.ID == position.ID {
					b.logger.Printf("Position %s already finalized in history; skipping completion", position.ID)
					return false
				}
			}
			b.logger.Printf("Position %s was closed by exit order, completing position close", position.ID)
			exitReason := strategy.ExitReason(position.ExitReason)
			b.completePositionClose(position, exitReason)
			return false
		}
		b.logger.Printf("Position %s is already closed, treating as no active position", position.ID)
		return false
	}

	// Calculate real-time P&L
	realTimePnL, err := b.strategy.CalculatePositionPnL(position)
	if err != nil {
		b.logger.Printf("Warning: Could not calculate real-time P&L: %v", err)
		realTimePnL = position.CurrentPnL // Fall back to stored value
	} else {
		delta := math.Abs(realTimePnL - position.CurrentPnL)
		if delta >= 0.01 {
			position.CurrentPnL = realTimePnL
			if err := b.storage.SetCurrentPosition(position); err != nil {
				b.logger.Printf("Warning: Failed to update position P&L: %v", err)
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
	placedOrder, err := b.broker.PlaceStrangleOrder(
		order.Symbol,
		order.PutStrike,
		order.CallStrike,
		order.Expiration,
		order.Quantity,
		order.Credit, // limit price
		false,        // not preview
		"day",        // duration
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

	b.logger.Printf("Position saved: ID=%s, Credit=$%.2f, DTE=%d",
		position.ID, position.CreditReceived, position.DTE)

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

	b.logger.Printf("Placing buy-to-close order with max debit: $%.2f", maxDebit)
	closeOrder, err := b.retryClient.ClosePositionWithRetry(ctx, position, maxDebit)
	if err != nil {
		b.logger.Printf("Failed to place close order after retries: %v", err)
		return
	}

	b.logger.Printf("Close order placed successfully: %d", closeOrder.Order.ID)

	// Store close order metadata and set position to closing state
	position.ExitOrderID = fmt.Sprintf("%d", closeOrder.Order.ID)
	position.ExitReason = string(reason)
	if err := position.TransitionState(models.StateAdjusting, "close_order_placed"); err != nil {
		b.logger.Printf("Warning: failed to transition to closing state: %v", err)
	}

	if err := b.storage.SetCurrentPosition(position); err != nil {
		b.logger.Printf("Failed to save position with close order ID: %v", err)
		return
	}

	b.logger.Printf("Position %s set to closing state, monitoring close order %d", position.ID, closeOrder.Order.ID)

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

	// Only allow exits from Open state or management states
	if currentState == models.StateOpen || isManagementState(currentState) {
		return true
	}

	// For all other states (including Submitted, Idle, Error, Adjusting, Rolling)
	b.logger.Printf("Position %s is in state %s, not eligible for close (only Open and management states allowed)",
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

	switch reason {
	case strategy.ExitReasonProfitTarget:
		return position.CreditReceived * 0.5
	case strategy.ExitReasonTime:
		if cvErr == nil {
			return currentVal / (float64(position.Quantity) * 100)
		}
		return position.CreditReceived
	case strategy.ExitReasonStopLoss:
		if cvErr == nil {
			return currentVal / (float64(position.Quantity) * 100)
		}
		return position.CreditReceived * b.config.Strategy.Exit.StopLossPct
	default:
		return position.CreditReceived * 1.0
	}
}

func (b *Bot) completePositionClose(
	position *models.Position,
	reason strategy.ExitReason,
) {
	actualPnL := b.calculateActualPnL(position, reason)
	b.logPnL(position, actualPnL)

	if err := position.TransitionState(models.StateClosed, "position_closed"); err != nil {
		b.logger.Printf("Warning: failed to transition to Closed: %v", err)
	}

	if err := b.storage.ClosePosition(actualPnL, string(reason)); err != nil {
		b.logger.Printf("Failed to close position in storage: %v", err)
		return
	}

	b.logger.Printf("Position closed successfully: %s", reason)

	stats := b.storage.GetStatistics()
	b.logger.Printf("Trade Statistics - Total: %d, Win Rate: %.1f%%, Total P&L: $%.2f",
		stats.TotalTrades, stats.WinRate*100, stats.TotalPnL)
}

func (b *Bot) calculateActualPnL(position *models.Position, reason strategy.ExitReason) float64 {
	actualPnL, err := b.strategy.CalculatePositionPnL(position)
	if err != nil {
		b.logger.Printf("Warning: Could not calculate real P&L, using estimated value: %v", err)
		if reason == strategy.ExitReasonProfitTarget {
			return position.CreditReceived * float64(position.Quantity) * 100 * 0.5
		}
		return position.CreditReceived * float64(position.Quantity) * 100 * 0.2
	}
	return actualPnL
}

func (b *Bot) logPnL(position *models.Position, actualPnL float64) {
	denom := position.CreditReceived * float64(position.Quantity) * 100
	if denom <= 0 {
		b.logger.Printf("Position P&L: $%.2f (credit unknown)", actualPnL)
		return
	}
	percent := (actualPnL / denom) * 100
	b.logger.Printf("Position P&L: $%.2f (%.1f%% of total credit received)", actualPnL, percent)
}

func (b *Bot) checkAdjustments() {
	b.logger.Println("Checking for adjustments...")
	// TODO: Implement adjustment logic (Phase 2)
}

// generatePositionID creates a simple unique ID for positions
func generatePositionID() string {
	// Simple ID generation using timestamp and random bytes
	now := time.Now().Format("20060102-150405")

	// Add 8 random bytes for uniqueness
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback if random fails
		return fmt.Sprintf("%s-%d", now, time.Now().UnixNano()%10000)
	}

	return fmt.Sprintf("%s-%x", now, randomBytes)
}
