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

	"github.com/eddie/spy-strangle-bot/internal/broker"
	"github.com/eddie/spy-strangle-bot/internal/config"
	"github.com/eddie/spy-strangle-bot/internal/models"
	"github.com/eddie/spy-strangle-bot/internal/storage"
	"github.com/eddie/spy-strangle-bot/internal/strategy"
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
		shouldExit, reason := b.strategy.CheckExitConditions()
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

	b.logger.Printf("Found existing position: %s %s Put %.0f / Call %.0f (DTE: %d, P&L: $%.2f)",
		position.Symbol, position.Expiration.Format("2006-01-02"),
		position.PutStrike, position.CallStrike,
		position.CalculateDTE(), position.CurrentPnL)

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
		order.Credit,
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
	
	// TODO: Set EntryIVR when IVR calculation is available
	position.EntryIVR = 35.0 // Placeholder
	
	// Initialize position state
	if err := position.TransitionState(models.StateOpen, "order_filled"); err != nil {
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
}

func (b *Bot) executeExit(reason string) {
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

	// TODO: Place buy-to-close order via broker
	// For now, we'll simulate the exit for storage/state management testing
	
	// Simulate current market value for P&L calculation
	// In real implementation, this would get current option quotes
	simulatedPnL := position.CreditReceived * 0.3 // Assume 30% profit for testing
	
	b.logger.Printf("Position P&L: $%.2f (%.1f%% of credit received)", 
		simulatedPnL, (simulatedPnL/position.CreditReceived)*100)

	// Close position in storage
	if err := b.storage.ClosePosition(simulatedPnL); err != nil {
		b.logger.Printf("Failed to close position in storage: %v", err)
		return
	}

	b.logger.Printf("Position closed successfully: %s", reason)
	
	// Log statistics
	stats := b.storage.GetStatistics()
	b.logger.Printf("Trade Statistics - Total: %d, Win Rate: %.1f%%, Total P&L: $%.2f",
		stats.TotalTrades, stats.WinRate*100, stats.TotalPnL)
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