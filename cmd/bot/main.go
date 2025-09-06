package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eddie/spy-strangle-bot/internal/broker"
	"github.com/eddie/spy-strangle-bot/internal/config"
	"github.com/eddie/spy-strangle-bot/internal/strategy"
)

type Bot struct {
	config   *config.Config
	broker   *broker.TradierClient
	strategy *strategy.StrangleStrategy
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
		cfg.Broker.APIEndpoint,
		cfg.Broker.AccountID,
		cfg.IsPaperTrading(),
	)

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
	// Check for saved position state
	// In MVP, this reads from positions.json
	// TODO: Implement position state management
	return false
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
	
	b.logger.Printf("Order placed successfully: %s", placedOrder.ID)
	
	// Save position state
	// TODO: Implement position persistence
}

func (b *Bot) executeExit(reason string) {
	b.logger.Printf("Executing exit: %s", reason)
	// TODO: Implement exit logic
}

func (b *Bot) checkAdjustments() {
	b.logger.Println("Checking for adjustments...")
	// TODO: Implement adjustment logic (Phase 2)
}