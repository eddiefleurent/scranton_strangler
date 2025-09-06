package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eddie/scranton_strangler/internal/broker"
	"github.com/eddie/scranton_strangler/internal/config"
	"github.com/eddie/scranton_strangler/internal/models"
	"github.com/eddie/scranton_strangler/internal/storage"
	"github.com/eddie/scranton_strangler/internal/strategy"
)

type Bot struct {
	config   *config.Config
	broker   *broker.TradierClient
	strategy *strategy.StrangleStrategy
	storage  storage.StorageInterface
	logger   *log.Logger
	stop     chan struct{}
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
	)

	// Initialize storage
	storage, err := storage.NewStorage("positions.json")
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	bot.storage = storage

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
			b.executeExit(reason)
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

func (b *Bot) executeExit(reason strategy.ExitReason) {
	b.logger.Printf("Executing exit: %s", reason)

	// Get current position
	position := b.storage.GetCurrentPosition()
	if position == nil {
		b.logger.Printf("No position to exit")
		return
	}

	b.logger.Printf("Closing position: %s %s Put %.0f / Call %.0f",
		position.Symbol, position.Expiration.Format("2006-01-02"),
		position.PutStrike, position.CallStrike)

	// Calculate maximum debit willing to pay (based on exit reason)
	var maxDebit float64
	switch reason {
	case strategy.ExitReasonProfitTarget:
		// For profit target, pay up to 50% of credit received
		maxDebit = position.CreditReceived * 0.5
	case strategy.ExitReasonTime:
		// For time-based exits, pay up to current market value
		maxDebit = position.CreditReceived * 2.0 // Max 2x credit as stop loss
	case strategy.ExitReasonStopLoss:
		// For stop loss, be more aggressive
		maxDebit = position.CreditReceived * 1.5 // Max 1.5x credit
	default:
		// Default conservative approach
		maxDebit = position.CreditReceived * 1.0
	}

	// Place buy-to-close order
	b.logger.Printf("Placing buy-to-close order with max debit: $%.2f", maxDebit)
	closeOrder, err := b.broker.CloseStranglePosition(
		position.Symbol,
		position.PutStrike,
		position.CallStrike,
		position.Expiration.Format("2006-01-02"),
		position.Quantity,
		maxDebit,
	)

	if err != nil {
		b.logger.Printf("Failed to place close order: %v", err)
		return
	}

	b.logger.Printf("Close order placed successfully: %d", closeOrder.Order.ID)

	// Calculate actual P&L using real-time quotes
	actualPnL, err := b.strategy.CalculatePositionPnL(position)
	if err != nil {
		b.logger.Printf("Warning: Could not calculate real P&L, using estimated value: %v", err)
		// Fall back to estimated P&L
		if reason == "50% profit target" {
			actualPnL = position.CreditReceived * 0.5 // 50% profit
		} else {
			actualPnL = position.CreditReceived * 0.2 // 20% profit for early exits
		}
	}

	b.logger.Printf("Position P&L: $%.2f (%.1f%% of credit received)",
		actualPnL, (actualPnL/position.CreditReceived)*100)

	// Close position in storage
	if err := b.storage.ClosePosition(actualPnL, string(reason)); err != nil {
		b.logger.Printf("Failed to close position in storage: %v", err)
		return
	}

	b.logger.Printf("Position closed successfully: %s", reason)

	// Log statistics
	stats := b.storage.GetStatistics()
	b.logger.Printf("Trade Statistics - Total: %d, Win Rate: %.1f%%, Total P&L: $%.2f",
		stats.TotalTrades, stats.WinRate*100, stats.TotalPnL)
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

	if err := position.TransitionState(models.StateClosed, "order_timeout"); err != nil {
		b.logger.Printf("Failed to transition position %s to closed: %v", positionID, err)
		return
	}

	if err := b.storage.SetCurrentPosition(position); err != nil {
		b.logger.Printf("Failed to save position %s after timeout: %v", positionID, err)
		return
	}

	b.logger.Printf("Position %s closed due to order timeout", positionID)
}

func (b *Bot) checkAdjustments() {
	b.logger.Println("Checking for adjustments...")
	// TODO: Implement adjustment logic (Phase 2)
}

// generatePositionID creates a simple unique ID for positions
func generatePositionID() string {
	// Simple ID generation using timestamp and random bytes
	now := time.Now().Format("20060102-150405")

	// Add 4 random bytes for uniqueness
	randomBytes := make([]byte, 2)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback if random fails
		return fmt.Sprintf("%s-%d", now, time.Now().UnixNano()%10000)
	}

	return fmt.Sprintf("%s-%x", now, randomBytes)
}
