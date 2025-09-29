// Package main provides the entry point for the SPY short strangle trading bot.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/dashboard"
	"github.com/eddiefleurent/scranton_strangler/internal/orders"
	"github.com/eddiefleurent/scranton_strangler/internal/retry"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
	"github.com/sirupsen/logrus"
)

// Bot represents the main trading bot instance.
type Bot struct {
	config        *config.Config
	broker        broker.Broker
	strategy      *strategy.StrangleStrategy
	storage       storage.Interface
	logger        *log.Logger
	dashLogger    *logrus.Logger
	dashServer    *dashboard.Server
	stop          chan struct{}
	ctx           context.Context // Main bot context for operations
	orderManager  *orders.Manager
	retryClient   *retry.Client
	nyLocation    *time.Location // Cached NY timezone location
	lastPnLUpdate time.Time      // Last time P&L was persisted to reduce write amplification
	pnlThrottle   time.Duration  // Minimum interval between P&L updates
	calendarMu    sync.RWMutex   // protects market calendar cache

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
		if os.Getenv("BOT_SKIP_LIVE_WAIT") != "1" {
			logger.Println("Waiting 10 seconds to confirm... (set BOT_SKIP_LIVE_WAIT=1 to skip)")
			time.Sleep(10 * time.Second)
		}
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

	// Initialize dashboard if enabled
	if cfg.Dashboard.Enabled {
		// Create logrus logger for dashboard
		dashLogger := logrus.New()
		dashLogger.SetOutput(os.Stdout)
		if cfg.Environment.Mode == "live" {
			dashLogger.SetFormatter(&logrus.JSONFormatter{})
		} else {
			dashLogger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
		}
		if lvl, err := logrus.ParseLevel(cfg.Environment.LogLevel); err == nil {
			dashLogger.SetLevel(lvl)
		}
		bot.dashLogger = dashLogger

		dashConfig := dashboard.Config{
			Port:                cfg.Dashboard.Port,
			AuthToken:           cfg.Dashboard.AuthToken,
			AllocationThreshold: cfg.Strategy.AllocationPct * 100, // Convert to percentage
			ProfitTarget:        cfg.Strategy.Exit.ProfitTarget,
			StopLossPct:         cfg.Strategy.Exit.StopLossPct,
		}
		bot.dashServer = dashboard.NewServer(dashConfig, bot.storage, bot.broker, bot.dashLogger)
		logger.Printf("Dashboard enabled at http://localhost:%d", cfg.Dashboard.Port)
	}

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

	// Start dashboard server if enabled
	if bot.dashServer != nil {
		go func() {
			if err := bot.dashServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Printf("Dashboard server error: %v", err)
			}
		}()
		
		// Ensure dashboard is shutdown gracefully
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := bot.dashServer.Shutdown(shutdownCtx); err != nil {
				logger.Printf("Error shutting down dashboard: %v", err)
			} else {
				logger.Println("Dashboard server stopped")
			}
		}()
	}

	// Run the bot
	if err := bot.Run(ctx); err != nil {
		logger.Printf("Bot error: %v", err)
		return
	}

	logger.Println("Bot stopped successfully")
}

// Run starts the bot's main execution loop.
func (b *Bot) Run(ctx context.Context) error {
	b.ctx = ctx // Store context for use in operations
	b.logger.Println("Bot starting main loop...")

	// Verify broker connection with timeout
	b.logger.Println("Verifying broker connection...")
	ctxBal, cancelBal := context.WithTimeout(ctx, 10*time.Second)
	defer cancelBal()
	
	bal, err := b.broker.GetAccountBalanceCtx(ctxBal)
	if err != nil {
		if ctxBal.Err() != nil {
			return fmt.Errorf("broker health check timed out: %w", ctxBal.Err())
		} else if ctx.Err() != nil {
			return fmt.Errorf("broker health check cancelled: %w", ctx.Err())
		} else {
			return fmt.Errorf("failed to connect to broker: %w", err)
		}
	}
	b.logger.Printf("Connected to broker. Account balance: $%.2f", bal)

	// Broker-first initialization: sync local storage with broker reality
	if err := b.performStartupReconciliation(ctx); err != nil {
		b.logger.Printf("Warning: Startup reconciliation failed: %v", err)
		b.logger.Printf("Continuing with existing local data...")
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
	// Use the new TradingCycle handler
	tradingCycle := NewTradingCycle(b)
	tradingCycle.Run()
}







// Utility functions have been moved to utils.go

// performStartupReconciliation syncs local storage with broker reality at startup
func (b *Bot) performStartupReconciliation(ctx context.Context) error {
	b.logger.Println("🔄 BROKER-FIRST RECONCILIATION: Syncing with broker reality...")
	
	// Get broker positions with timeout
	auditCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	brokerPositions, err := b.broker.GetPositionsCtx(auditCtx)
	if err != nil {
		return fmt.Errorf("failed to get broker positions: %w", err)
	}
	
	// Get current local positions
	localPositions := b.storage.GetCurrentPositions()
	
	b.logger.Printf("Broker positions: %d, Local positions: %d", len(brokerPositions), len(localPositions))
	
	// Simple check: if we have zero-credit positions, we have corruption
	hasCorruption := false
	for _, pos := range localPositions {
		if pos.CreditReceived <= 0.01 {
			hasCorruption = true
			b.logger.Printf("Found corrupted position with zero credit: %s", pos.ID)
			break
		}
	}
	
	// If we have broker positions but corruption or empty local storage, reset
	if len(brokerPositions) > 0 && (len(localPositions) == 0 || hasCorruption) {
		b.logger.Printf("🧹 Corruption detected - will use manual cleanup tools")
		b.logger.Printf("Run './audit -v' and './reset_positions' to fix automatically")
		
		// Don't auto-fix for now - require manual intervention
		// This is safer until we fully trust the auto-reconciliation
	} else {
		b.logger.Printf("✅ Local storage appears consistent with broker")
	}
	
	return nil
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
	b.calendarMu.RLock()
	if b.marketCalendar != nil &&
		b.calendarCacheMonth == month &&
		b.calendarCacheYear == year {
		cal := b.marketCalendar
		b.calendarMu.RUnlock()
		return cal, nil
	}
	b.calendarMu.RUnlock()

	// Fetch new calendar data
	b.logger.Printf("Fetching market calendar for %d/%d", month, year)
	
	// Use context with timeout for the API call
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	calendar, err := b.broker.GetMarketCalendarCtx(ctx, month, year)
	if err != nil {
		return nil, fmt.Errorf("failed to get market calendar: %w", err)
	}

	// Cache the result (note: nil does not count as a cache hit)
	b.calendarMu.Lock()
	b.marketCalendar = calendar
	b.calendarCacheMonth = month
	b.calendarCacheYear = year
	b.calendarMu.Unlock()

	// Compute safe daysCount with proper checks
	daysCount := 0
	if calendar != nil && calendar.Calendar.Days.Day != nil {
		daysCount = len(calendar.Calendar.Days.Day)
	}

	b.logger.Printf("Cached market calendar: %d days for %d/%d", 
		daysCount, month, year)

	return calendar, nil
}

// getTodaysMarketSchedule gets today's market schedule from the cached calendar
func (b *Bot) getTodaysMarketSchedule() (*broker.MarketDay, error) {
	var now time.Time
	if b.nyLocation != nil {
		now = time.Now().In(b.nyLocation)
	} else {
		now = time.Now().In(time.UTC)
	}
	calendar, err := b.getMarketCalendar(int(now.Month()), now.Year())
	if err != nil {
		return nil, err
	}

	// Find today's schedule
	today := now.Format("2006-01-02")
	if calendar == nil || calendar.Calendar.Days.Day == nil || len(calendar.Calendar.Days.Day) == 0 {
		return nil, fmt.Errorf("broker returned empty calendar payload for %s", today)
	}
	for _, day := range calendar.Calendar.Days.Day {
		if day.Date == today {
			return &day, nil
		}
	}

	// Today's data not found in cache - force refresh and try again
	b.logger.Printf("Today's date %s not found in cached calendar, forcing refresh", today)
	b.calendarMu.Lock()
	b.marketCalendar = nil // Clear cache to force refresh
	b.calendarMu.Unlock()

	calendar, err = b.getMarketCalendar(int(now.Month()), now.Year())
	if err != nil {
		return nil, fmt.Errorf("failed to refresh calendar: %w", err)
	}

	// Try again with fresh data
	if calendar == nil || calendar.Calendar.Days.Day == nil || len(calendar.Calendar.Days.Day) == 0 {
		return nil, fmt.Errorf("broker returned empty calendar payload for %s after refresh", today)
	}
	for _, day := range calendar.Calendar.Days.Day {
		if day.Date == today {
			return &day, nil
		}
	}

	return nil, fmt.Errorf("today's date %s still not found after calendar refresh", today)
}

