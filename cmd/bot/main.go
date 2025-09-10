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
	"strconv"
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
	
	// Market calendar caching
	marketCalendar     *broker.MarketCalendarResponse
	calendarCacheMonth int
	calendarCacheYear  int
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
		DTERange:            cfg.Strategy.Entry.DTERange,
		DeltaTarget:         cfg.Strategy.Entry.Delta / 100, // Convert from percentage
		ProfitTarget:        cfg.Strategy.Exit.ProfitTarget,
		MaxDTE:              cfg.Strategy.Exit.MaxDTE,
		AllocationPct:       cfg.Strategy.AllocationPct,
		MinIVPct:            cfg.Strategy.Entry.MinIVPct,
		MinCredit:           cfg.Strategy.Entry.MinCredit,
		EscalateLossPct:     cfg.Strategy.EscalateLossPct,
		StopLossPct:         cfg.Strategy.Exit.StopLossPct,
		MaxPositionLoss:     cfg.Risk.MaxPositionLoss,
		MaxContracts:        cfg.Risk.MaxContracts,
		MinVolume:           cfg.Strategy.Entry.MinVolume,
		MinOpenInterest:     cfg.Strategy.Entry.MinOpenInterest,
	}
	bot.strategy = strategy.NewStrangleStrategy(bot.broker, strategyConfig, logger, bot.storage)

	// Initialize order manager
	bot.orderManager = orders.NewManager(bot.broker, bot.storage, logger, bot.stop)

	// Initialize retry client
	bot.retryClient = retry.NewClient(bot.broker, logger)

	// Pre-fetch this month's market calendar for caching
	logger.Println("Fetching market calendar for this month...")
	_, calErr := bot.getMarketCalendar(0, 0) // Current month/year
	if calErr != nil {
		logger.Printf("Warning: Could not fetch market calendar: %v (will fallback to real-time checks)", calErr)
	}

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
	type balanceResult struct {
		balance float64
		err     error
	}
	resCh := make(chan balanceResult, 1)

	// Add cancellation for balance fetch to avoid potential startup goroutine leak
	ctxBal, cancelBal := context.WithTimeout(ctx, 10*time.Second)
	defer cancelBal()
	go func() {
		// Try context-aware method first, fallback to regular method if not available
		type balFn interface{ GetAccountBalanceCtx(context.Context) (float64, error) }
		if cb, ok := b.broker.(balFn); ok {
			bal, err := cb.GetAccountBalanceCtx(ctxBal)
			resCh <- balanceResult{balance: bal, err: err}
			return
		}
		// Fallback: keep existing call (cannot cancel)
		bal, err := b.broker.GetAccountBalance()
		resCh <- balanceResult{balance: bal, err: err}
	}()

	select {
	case res := <-resCh:
		if res.err != nil {
			return fmt.Errorf("failed to connect to broker: %w", res.err)
		}
		b.logger.Printf("Connected to broker. Account balance: $%.2f", res.balance)
	case <-time.After(10 * time.Second):
		return fmt.Errorf("broker health check timed out after 10 seconds")
	case <-ctx.Done():
		return fmt.Errorf("broker health check cancelled: %w", ctx.Err())
	}

	// Main trading loop
	interval := b.config.GetCheckInterval()
	if interval <= 0 {
		b.logger.Printf("Warning: invalid check interval %v; defaulting to 30s", interval)
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
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

	// Get today's official market schedule (cached monthly)
	todaySchedule, schedErr := b.getTodaysMarketSchedule()
	if schedErr != nil {
		b.logger.Printf("Warning: Could not get today's market schedule: %v", schedErr)
	} else {
		// Log today's official schedule
		if todaySchedule.Status == "closed" {
			b.logger.Printf("Market is officially CLOSED today: %s", todaySchedule.Description)
			b.logger.Println("Trading cycle skipped - market holiday")
			return
		}
		if todaySchedule.Open != nil {
			b.logger.Printf("Official market hours today: %s - %s", 
				todaySchedule.Open.Start, todaySchedule.Open.End)
		}
	}

	// Check real-time market status from Tradier
	marketClock, err := b.broker.GetMarketClock(false)
	if err != nil {
		b.logger.Printf("Warning: Could not get market clock: %v, falling back to config-based hours", err)
	}

	// Determine market status
	var isMarketOpen bool
	var marketState string
	
	if marketClock != nil {
		marketState = marketClock.Clock.State
		// Tradier states: "open", "closed", "premarket", "postmarket"
		isMarketOpen = marketState == "open"
		b.logger.Printf("Real-time market status: %s", marketState)
	} else {
		// Fallback to config-based hours
		isMarketOpen = b.config.IsWithinTradingHours(now)
		marketState = "unknown"
		b.logger.Printf("Using config-based market hours: open=%t", isMarketOpen)
	}

	// Handle non-trading hours
	if !isMarketOpen {
		if !b.config.Schedule.AfterHoursCheck {
			b.logger.Printf("Market is %s, skipping cycle", marketState)
			return
		}
		b.logger.Printf("Market is %s, running after-hours check for existing positions only", marketState)
	}

	b.logger.Println("Starting trading cycle...")

	// Get all current positions (supports multiple positions)
	positions := b.storage.GetCurrentPositions()
	b.logger.Printf("Currently managing %d position(s)", len(positions))

	// Check exit conditions for each position
	for _, position := range positions {
		b.logger.Printf("Checking position %s (%.2f/%.2f, %s DTE)", 
			position.ID[:8], position.PutStrike, position.CallStrike, position.Expiration.Format("2006-01-02"))
		
		// Create a copy to work with
		posCopy := position
		shouldExit, reason := b.strategy.CheckExitConditions(&posCopy)
		if shouldExit {
			b.logger.Printf("Exit signal for position %s: %s", position.ID[:8], reason)
			b.executeExitForPosition(&posCopy, reason)
		} else {
			b.logger.Printf("No exit conditions met for position %s", position.ID[:8])
		}

		// Check for adjustments (Phase 2) - only during regular hours
		if b.config.Strategy.Adjustments.Enabled && isMarketOpen {
			b.checkAdjustmentsForPosition(&posCopy)
		}
	}

	// Check if we can open new positions - only during regular trading hours
	maxPositions := b.config.Risk.MaxPositions
	if maxPositions <= 0 {
		maxPositions = 1 // Default to 1 if not configured
	}
	
	if isMarketOpen && len(positions) < maxPositions {
		b.logger.Printf("Have %d/%d positions, checking entry conditions...", len(positions), maxPositions)
		
		// Check if we have sufficient buying power for new positions
		buyingPower, err := b.broker.GetOptionBuyingPower()
		if err != nil {
			b.logger.Printf("Warning: Could not get option buying power: %v", err)
			buyingPower = 0
		}
		
		b.logger.Printf("Available option buying power: $%.2f", buyingPower)
		
		// Only attempt entry if we have meaningful buying power
		if buyingPower > 1000 { // Minimum threshold for a new position
			canEnter, reason := b.strategy.CheckEntryConditions()
			if canEnter {
				b.logger.Printf("Entry signal: %s", reason)
				// Try to open positions up to the max limit
				remainingSlots := maxPositions - len(positions)
				for i := 0; i < remainingSlots && i < 1; i++ { // Limit to 1 new position per cycle for safety
					b.executeEntry()
				}
			} else {
				b.logger.Printf("Entry conditions not met: %s", reason)
			}
		} else {
			b.logger.Printf("Insufficient buying power for new positions")
		}
	} else if !isMarketOpen {
		b.logger.Printf("Market %s: Skipping entry checks, only monitoring existing positions", marketState)
	} else {
		b.logger.Printf("Maximum positions (%d) reached, not checking for new entries", maxPositions)
	}


	b.logger.Println("Trading cycle complete")
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

	px := math.Max(util.FloorToTick(order.Credit, tickSize), tickSize)
	b.logger.Printf("Using tick size %.4f for symbol %s, rounded price: $%.2f", tickSize, order.Symbol, px)

	// Generate deterministic client-order ID for potential deduplication
	canonicalString := fmt.Sprintf("entry-%s-%s-%.2f-%.2f-%d-%.2f",
		order.Symbol,
		order.Expiration,
		order.PutStrike,
		order.CallStrike,
		order.Quantity,
		px)

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

	// Set real entry IV percentage
	position.EntryIV = b.strategy.GetCurrentIV() // SPY ATM IV as percentage

	// Set the broker order ID
	position.EntryOrderID = fmt.Sprintf("%d", placedOrder.Order.ID)

	// Initialize position state to submitted
	if err := position.TransitionState(models.StateSubmitted, "order_placed"); err != nil {
		b.logger.Printf("Failed to set position state: %v", err)
		return
	}

	// Save position to storage
	if err := b.storage.AddPosition(position); err != nil {
		b.logger.Printf("Failed to save position: %v", err)
		return
	}

	b.logger.Printf("Position saved: ID=%s, LimitPrice=$%.2f, DTE=%d",
		position.ID, position.EntryLimitPrice, position.DTE)

	// Start order status polling in background
	go b.orderManager.PollOrderStatus(position.ID, placedOrder.Order.ID, true)
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

	// Enable idempotent re-attempts from Adjusting once prior close is terminal
	if currentState == models.StateAdjusting {
		// Check if there's no active exit order ID (order was never placed or was cleared)
		if position.ExitOrderID == "" {
			b.logger.Printf("Position %s in Adjusting state with no active exit order, allowing re-attempt", position.ID)
			return true
		}

		// Check if the prior close order has terminal status (filled/canceled/rejected)
		if position.ExitOrderID != "" {
			orderID, err := strconv.Atoi(position.ExitOrderID)
			if err != nil {
				b.logger.Printf("Position %s has invalid ExitOrderID %s: %v", position.ID, position.ExitOrderID, err)
				return false
			}

			isTerminal, err := b.orderManager.IsOrderTerminal(b.ctx, orderID)
			if err != nil {
				b.logger.Printf("Failed to check order status for %s: %v, blocking re-attempt to be safe", position.ExitOrderID, err)
				return false
			}

			if isTerminal {
				b.logger.Printf("Position %s prior close order %s is terminal, allowing idempotent re-attempt", position.ID, position.ExitOrderID)
				// Clear the terminal order ID to allow re-attempt
				position.ExitOrderID = ""
				position.ExitReason = ""
				// Use UpdatePosition to persist to both legacy and CurrentPositions
				if err := b.storage.UpdatePosition(position); err != nil {
					b.logger.Printf("Warning: Failed to clear terminal exit order ID: %v", err)
				}
				return true
			}
		}

		// Prior order is still active, block duplicate attempts
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





// generatePositionID creates a unique ID for positions using UUID for guaranteed uniqueness
func generatePositionID() string {
	// Use UUID for guaranteed uniqueness
	return uuid.New().String()
}

// getMarketCalendar gets the market calendar for a given month/year, with caching
func (b *Bot) getMarketCalendar(month, year int) (*broker.MarketCalendarResponse, error) {
	// Use current month/year if not specified
	// Use NY timezone when defaulting month/year for calendar cache
	now := time.Now()
	if b.nyLocation != nil {
		now = now.In(b.nyLocation)
	}
	if month == 0 {
		month = int(now.Month())
	}
	if year == 0 {
		year = now.Year()
	}

	// Check if we have cached data for this month/year
	if b.marketCalendar != nil && 
		b.calendarCacheMonth == month && 
		b.calendarCacheYear == year {
		return b.marketCalendar, nil
	}

	// Fetch new calendar data
	b.logger.Printf("Fetching market calendar for %d/%d", month, year)
	calendar, err := b.broker.GetMarketCalendar(month, year)
	if err != nil {
		return nil, fmt.Errorf("failed to get market calendar: %w", err)
	}

	// Cache the result
	b.marketCalendar = calendar
	b.calendarCacheMonth = month
	b.calendarCacheYear = year

	b.logger.Printf("Cached market calendar: %d days for %d/%d", 
		len(calendar.Calendar.Days.Day), month, year)

	return calendar, nil
}

// getTodaysMarketSchedule gets today's market schedule from the cached calendar
func (b *Bot) getTodaysMarketSchedule() (*broker.MarketDay, error) {
	now := time.Now().In(b.nyLocation)
	calendar, err := b.getMarketCalendar(int(now.Month()), now.Year())
	if err != nil {
		return nil, err
	}

	// Find today's schedule
	today := now.Format("2006-01-02")
	for _, day := range calendar.Calendar.Days.Day {
		if day.Date == today {
			return &day, nil
		}
	}

	// Today's data not found in cache - force refresh and try again
	b.logger.Printf("Today's date %s not found in cached calendar, forcing refresh", today)
	b.marketCalendar = nil // Clear cache to force refresh

	calendar, err = b.getMarketCalendar(int(now.Month()), now.Year())
	if err != nil {
		return nil, fmt.Errorf("failed to refresh calendar: %w", err)
	}

	// Try again with fresh data
	for _, day := range calendar.Calendar.Days.Day {
		if day.Date == today {
			return &day, nil
		}
	}

	return nil, fmt.Errorf("today's date %s still not found after calendar refresh", today)
}

// executeExitForPosition executes exit for a specific position
func (b *Bot) executeExitForPosition(position *models.Position, reason strategy.ExitReason) {
	b.logger.Printf("Executing exit for position %s: %s", position.ID[:8], reason)

	if !b.isPositionReadyForExit(position) {
		return
	}

	b.logPositionClose(position)

	maxDebit := b.calculateMaxDebit(position, reason)

	if maxDebit <= 0 {
		b.logger.Printf("Skipping close order for position %s: calculated maxDebit $%.2f is invalid (must be > 0)", position.ID[:8], maxDebit)
		return
	}

	// Get appropriate tick size for the symbol and round maxDebit up to tick size
	tickSize, err := b.broker.GetTickSize(position.Symbol)
	if err != nil {
		b.logger.Printf("Warning: Failed to get tick size for %s, using default 0.01: %v", position.Symbol, err)
		tickSize = 0.01
	}
	maxDebit = util.CeilToTick(maxDebit, tickSize)

	// Place the close order
	closeOrder, err := b.retryClient.ClosePositionWithRetry(
		b.ctx,
		position,
		maxDebit,
	)

	if err != nil {
		b.logger.Printf("Failed to place close order for position %s: %v", position.ID[:8], err)
		return
	}

	// Update position with exit order ID
	position.ExitOrderID = fmt.Sprintf("%d", closeOrder.Order.ID)
	if err := b.storage.UpdatePosition(position); err != nil {
		b.logger.Printf("Failed to update position %s with exit order ID: %v", position.ID[:8], err)
	}

	b.logger.Printf("Close order placed for position %s: order_id=%d, max_debit=$%.2f",
		position.ID[:8], closeOrder.Order.ID, maxDebit)

	// Start order status polling in background
	go b.orderManager.PollOrderStatus(position.ID, closeOrder.Order.ID, false)
}

// checkAdjustmentsForPosition checks if adjustments are needed for a specific position
func (b *Bot) checkAdjustmentsForPosition(position *models.Position) {
	// Placeholder for adjustment logic (Phase 2)
	// This would check if the position needs to be adjusted based on
	// market movement and the football system rules
	b.logger.Printf("Adjustment check for position %s not yet implemented", position.ID[:8])
}
