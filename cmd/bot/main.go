package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
)

type Bot struct {
	config   *config.Config
	broker   broker.Broker
	strategy *strategy.StrangleStrategy
	storage  storage.StorageInterface
	logger   *log.Logger
	stop     chan struct{}
	ctx      context.Context // Main bot context for operations
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
	bot.broker = broker.NewTradierClient(
		cfg.Broker.APIKey,
		cfg.Broker.AccountID,
		cfg.IsPaperTrading(),
		cfg.Broker.UseOTOCO,
		cfg.Strategy.Exit.ProfitTarget,
	)

	// Initialize storage
	store, err := storage.NewStorage("positions.json")
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	bot.storage = store

	// Initialize strategy
	strategyConfig := &strategy.StrategyConfig{
		Symbol:        cfg.Strategy.Symbol,
		DTETarget:     cfg.Strategy.Entry.TargetDTE,
		DeltaTarget:   cfg.Strategy.Entry.Delta / 100, // Convert from percentage
		ProfitTarget:  cfg.Strategy.Exit.ProfitTarget,
		MaxDTE:        cfg.Strategy.Exit.MaxDTE,
		AllocationPct: cfg.Strategy.AllocationPct,
		MinIVR:        cfg.Strategy.Entry.MinIVR,
		MinCredit:     cfg.Strategy.Entry.MinCredit,
	}
	bot.strategy = strategy.NewStrangleStrategy(bot.broker, strategyConfig)

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
	if !b.config.IsWithinTradingHours(now) {
		b.logger.Printf("Outside trading hours (%s - %s), skipping cycle",
			b.config.Schedule.TradingStart, b.config.Schedule.TradingEnd)
		return
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

		// Check for adjustments (Phase 2)
		if b.config.Strategy.Adjustments.Enabled {
			b.checkAdjustments()
		}
	} else {
		// Check entry conditions
		b.logger.Println("No position, checking entry conditions...")
		canEnter, reason := b.strategy.CheckEntryConditions()
		if canEnter {
			b.logger.Printf("Entry signal: %s", reason)
			b.executeEntry()
		} else {
			b.logger.Printf("Entry conditions not met: %s", reason)
		}
	}

	b.logger.Println("Trading cycle complete")
}

func (b *Bot) checkExistingPosition() bool {
	position := b.storage.GetCurrentPosition()
	if position == nil {
		return false
	}

	// Calculate real-time P&L
	realTimePnL, err := b.strategy.CalculatePositionPnL(position)
	if err != nil {
		b.logger.Printf("Warning: Could not calculate real-time P&L: %v", err)
		realTimePnL = position.CurrentPnL // Fall back to stored value
	} else {
		// Update stored P&L with real-time value
		position.CurrentPnL = realTimePnL
		if err := b.storage.SetCurrentPosition(position); err != nil {
			b.logger.Printf("Warning: Failed to update position P&L: %v", err)
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
	)

	if err != nil {
		b.logger.Printf("Failed to place order: %v", err)
		return
	}

	b.logger.Printf("Order placed successfully: %d", placedOrder.Order.ID)

	// Save position state
	positionID := generatePositionID()

	// Parse expiration string to time.Time
	expirationTime, err := time.Parse("2006-01-02", order.Expiration)
	if err != nil {
		b.logger.Printf("Failed to parse expiration date: %v", err)
		return
	}

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
	go b.pollOrderStatus(position.ID, placedOrder.Order.ID)
}

// closePositionWithRetry attempts to close a position with timeout and retry logic
func (b *Bot) closePositionWithRetry(
	ctx context.Context,
	position *models.Position,
	maxDebit float64,
	maxRetries int,
) (*broker.OrderResponse, error) {
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
		timeout        = 2 * time.Minute
	)

	closeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-closeCtx.Done():
			return nil, fmt.Errorf("close operation timed out after %v: %w", timeout, closeCtx.Err())
		default:
		}

		// Check if context is canceled
		if ctx.Err() != nil {
			return nil, fmt.Errorf("operation canceled: %w", ctx.Err())
		}

		b.logger.Printf("Close attempt %d/%d for position %s", attempt+1, maxRetries+1, position.ID)

		closeOrder, err := b.broker.CloseStranglePosition(
			position.Symbol,
			position.PutStrike,
			position.CallStrike,
			position.Expiration.Format("2006-01-02"),
			position.Quantity,
			maxDebit,
		)

		if err == nil {
			b.logger.Printf("Close order placed successfully on attempt %d: %d", attempt+1, closeOrder.Order.ID)
			return closeOrder, nil
		}

		lastErr = err
		b.logger.Printf("Close attempt %d failed: %v", attempt+1, err)

		// Check if error is transient (network, rate limit, temporary server error)
		if b.isTransientError(err) && attempt < maxRetries {
			b.logger.Printf("Transient error detected, retrying in %v", backoff)
			select {
			case <-time.After(backoff):
				// Exponential backoff with jitter
				backoff = time.Duration(float64(backoff) * 1.5)
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				// Add some jitter to avoid thundering herd
				maxJitter := int64(backoff / 4)
				if maxJitter > 0 {
					jitterVal, err := rand.Int(rand.Reader, big.NewInt(maxJitter))
					if err != nil {
						log.Printf("Failed to generate jitter: %v", err)
					} else {
						jitter := time.Duration(jitterVal.Int64())
						backoff += jitter
					}
				}
			case <-closeCtx.Done():
				return nil, fmt.Errorf("close operation timed out during backoff: %w", closeCtx.Err())
			case <-ctx.Done():
				return nil, fmt.Errorf("operation canceled during backoff: %w", ctx.Err())
			}
		} else {
			// Non-transient error or max retries reached
			break
		}
	}

	return nil, fmt.Errorf("failed to close position after %d attempts: %w", maxRetries+1, lastErr)
}

// isTransientError determines if an error is likely transient and worth retrying
func (b *Bot) isTransientError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Common transient error patterns
	transientPatterns := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"server error",
		"rate limit",
		"429", // HTTP 429 Too Many Requests
		"502", // HTTP 502 Bad Gateway
		"503", // HTTP 503 Service Unavailable
		"504", // HTTP 504 Gateway Timeout
		"network",
		"dns",
		"tcp",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
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

	_, err := b.placeCloseOrder(ctx, position, maxDebit)
	if err != nil {
		return
	}

	b.completePositionClose(position, reason)
}

func (b *Bot) isPositionReadyForExit(position *models.Position) bool {
	currentState := position.GetCurrentState()
	if currentState == models.StateClosed {
		b.logger.Printf("Position %s is already closed, skipping duplicate close attempt", position.ID)
		return false
	}

	if currentState == models.StateError || currentState == models.StateAdjusting || currentState == models.StateRolling {
		b.logger.Printf("Position %s is in state %s, not eligible for close", position.ID, currentState)
		return false
	}

	return true
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
		return position.CreditReceived * config.StopLossPct
	default:
		return position.CreditReceived * 1.0
	}
}

func (b *Bot) placeCloseOrder(
	ctx context.Context,
	position *models.Position,
	maxDebit float64,
) (*broker.OrderResponse, error) {
	b.logger.Printf("Placing buy-to-close order with max debit: $%.2f", maxDebit)

	closeOrder, err := b.closePositionWithRetry(ctx, position, maxDebit, 3)
	if err != nil {
		b.logger.Printf("Failed to place close order after retries: %v", err)
		return nil, err
	}

	b.logger.Printf("Close order placed successfully: %d", closeOrder.Order.ID)
	return closeOrder, nil
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
	if denom == 0 {
		denom = position.CreditReceived * 100
	}
	b.logger.Printf("Position P&L: $%.2f (%.1f%% of total credit received)",
		actualPnL, (actualPnL/denom)*100)
}

// pollOrderStatus polls the broker for order status until filled or timeout
func (b *Bot) pollOrderStatus(positionID string, orderID int) {
	const (
		pollInterval = 5 * time.Second // Check every 5 seconds
		timeout      = 5 * time.Minute // Give up after 5 minutes
	)

	b.logger.Printf("Starting order status polling for position %s, order %d", positionID, orderID)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.logger.Printf("Order polling timeout for position %s", positionID)
			b.handleOrderTimeout(positionID)
			return
		case <-b.stop:
			b.logger.Printf("Shutdown signal received during order polling for position %s", positionID)
			return
		case <-ticker.C:
			// Check order status
			orderStatus, err := b.broker.GetOrderStatus(orderID)
			if err != nil {
				b.logger.Printf("Error checking order status for %s: %v", positionID, err)
				continue
			}

			b.logger.Printf("Order %d status: %s", orderID, orderStatus.Order.Status)

			switch orderStatus.Order.Status {
			case "filled":
				b.logger.Printf("Order filled for position %s", positionID)
				b.handleOrderFilled(positionID)
				return
			case "canceled", "rejected":
				b.logger.Printf("Order failed for position %s: %s", positionID, orderStatus.Order.Status)
				b.handleOrderFailed(positionID, orderStatus.Order.Status)
				return
			case "pending", "open", "partial":
				// Continue polling
				continue
			default:
				b.logger.Printf("Unknown order status for position %s: %s", positionID, orderStatus.Order.Status)
			}
		}
	}
}

// handleOrderFilled transitions position to Open state when order is confirmed filled
func (b *Bot) handleOrderFilled(positionID string) {
	position := b.storage.GetCurrentPosition()
	if position == nil || position.ID != positionID {
		b.logger.Printf("Position %s not found or mismatched", positionID)
		return
	}

	if err := position.TransitionState(models.StateOpen, "order_filled"); err != nil {
		b.logger.Printf("Failed to transition position %s to open: %v", positionID, err)
		return
	}

	if err := b.storage.SetCurrentPosition(position); err != nil {
		b.logger.Printf("Failed to save position %s after fill: %v", positionID, err)
		return
	}

	b.logger.Printf("Position %s successfully transitioned to open state", positionID)
}

// handleOrderFailed transitions position to error state when order fails
func (b *Bot) handleOrderFailed(positionID string, reason string) {
	position := b.storage.GetCurrentPosition()
	if position == nil || position.ID != positionID {
		b.logger.Printf("Position %s not found or mismatched", positionID)
		return
	}

	if err := position.TransitionState(models.StateError, fmt.Sprintf("order_%s", reason)); err != nil {
		b.logger.Printf("Failed to transition position %s to error: %v", positionID, err)
		return
	}

	if err := b.storage.SetCurrentPosition(position); err != nil {
		b.logger.Printf("Failed to save position %s after failure: %v", positionID, err)
		return
	}

	b.logger.Printf("Position %s marked as error due to order failure: %s", positionID, reason)
}

// handleOrderTimeout transitions position to closed state when order times out
func (b *Bot) handleOrderTimeout(positionID string) {
	position := b.storage.GetCurrentPosition()
	if position == nil || position.ID != positionID {
		b.logger.Printf("Position %s not found or mismatched", positionID)
		return
	}

	if err := position.TransitionState(models.StateError, "order_timeout"); err != nil {
		b.logger.Printf("Failed to transition position %s to error: %v", positionID, err)
		return
	}

	if err := b.storage.SetCurrentPosition(position); err != nil {
		b.logger.Printf("Failed to save position %s after timeout: %v", positionID, err)
		return
	}

	b.logger.Printf("Position %s marked as error due to order timeout", positionID)
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
