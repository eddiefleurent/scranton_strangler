// Package main provides the entry point for the SPY short strangle trading bot.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/dashboard"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/orders"
	"github.com/eddiefleurent/scranton_strangler/internal/retry"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
	"github.com/sirupsen/logrus"
)

const (
	// MinCreditThreshold is the minimum credit required for a valid position (in dollars)
	MinCreditThreshold = 0.01

	// Option symbol parsing constants
	symbolBaseLength    = 3  // Length of base symbol (e.g., "SPY")
	symbolDateLength    = 6  // Length of YYMMDD date
	symbolPrefixLength  = 9  // Base + Date (SPY + YYMMDD)
	symbolTypeOffset    = 9  // Position of option type (C/P)
	symbolStrikeOffset  = 10 // Position where strike price begins
	strikeScaleDivisor  = 1000.0 // Divisor to convert strike from integer to decimal

	// Option types
	optionTypeCall = "call"
	optionTypePut  = "put"
)

// reconciliationResult contains the analysis of broker vs local position differences
type reconciliationResult struct {
	brokerOnlyPositions []string // Position symbols only in broker
	localOnlyPositions  []string // Position IDs only in local storage
	corruptedPositions  []string // Local position IDs with invalid credit
	hasInconsistencies  bool     // Whether any inconsistencies were found
}

// generateCorrelationID creates a short correlation ID for request tracking
func generateCorrelationID(logger *log.Logger) string {
	bytes := make([]byte, 4)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback to deterministic-but-unique ID if crypto/rand fails
		fallbackID := fmt.Sprintf("%x%x", time.Now().UnixNano(), os.Getpid())
		logger.Printf("Warning: crypto/rand.Read failed (%v), using fallback correlation ID", err)
		return fallbackID[:8] // Keep it short like the original
	}
	return hex.EncodeToString(bytes)
}

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
	os.Exit(run())
}

func run() int {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return 1
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

	// Cache NY timezone location with fallback
	if loc, err := time.LoadLocation("America/New_York"); err != nil {
		log.Printf("WARNING: Failed to load America/New_York timezone (%v), using EST fallback", err)
		bot.nyLocation = time.FixedZone("EST", -5*60*60) // EST: UTC-5
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
		log.Printf("Failed to create Tradier client: %v", err)
		return 1
	}

	// Wrap with circuit breaker for resilience
	bot.broker = broker.NewCircuitBreakerBroker(tradierClient)

	// Initialize storage
	storagePath := cfg.Storage.Path
	store, err := storage.NewStorage(storagePath)
	if err != nil {
		log.Printf("Failed to initialize storage: %v", err)
		return 1
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
		} else {
			dashLogger.SetLevel(logrus.InfoLevel)
			dashLogger.WithError(err).Warn("invalid log level; defaulting to info")
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
		logger.Printf("Dashboard enabled at http://0.0.0.0:%d (accessible via localhost:%d)", cfg.Dashboard.Port, cfg.Dashboard.Port)
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
				// Emit metric for monitoring dashboard failures
				logger.Printf("METRIC: dashboard_server_failures=1")
			}
		}()

		// Ensure dashboard is shutdown gracefully
		defer func() {
			// Shutdown is nil-safe internally, so we can always call it
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
		return 1
	}

	logger.Println("Bot stopped successfully")
	return 0
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
		correlationID := generateCorrelationID(b.logger)
		b.logger.Printf("Warning: Startup reconciliation failed: %v (correlation_id=%s)", err, correlationID)
		b.logger.Printf("Continuing with existing local data...")

		// Emit structured metric for monitoring
		b.logger.Printf("METRIC: reconciliation_failures=1 correlation_id=%s", correlationID)
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

	// Perform detailed reconciliation analysis
	result := b.analyzePositionDifferences(brokerPositions, localPositions)

	if result.hasInconsistencies {
		b.logger.Printf("⚠️  POSITION INCONSISTENCIES DETECTED - Initiating self-healing...")

		// Auto-heal phantom positions (local-only positions)
		if err := b.cleanupPhantomPositions(result.localOnlyPositions); err != nil {
			b.logger.Printf("❌ Failed to cleanup phantom positions: %v", err)
		}

		// Auto-recover untracked positions (broker-only positions)
		if err := b.recoverUntrackedPositions(ctx, brokerPositions, result.brokerOnlyPositions); err != nil {
			b.logger.Printf("❌ Failed to recover untracked positions: %v", err)
		}

		// Re-analyze after healing to check for any remaining issues
		localPositionsAfterHealing := b.storage.GetCurrentPositions()
		finalResult := b.analyzePositionDifferences(brokerPositions, localPositionsAfterHealing)

		// Log final state after healing
		if finalResult.hasInconsistencies {
			b.logger.Printf("⚠️  Some inconsistencies remain after self-healing:")
			b.logReconciliationIssues(finalResult)
		} else {
			b.logger.Printf("✅ Self-healing complete - local storage is now consistent with broker")
		}
	} else {
		b.logger.Printf("✅ Local storage is consistent with broker")
	}

	return nil
}

// analyzePositionDifferences computes set differences between broker and local positions
func (b *Bot) analyzePositionDifferences(brokerPositions []broker.PositionItem, localPositions []models.Position) reconciliationResult {
	result := reconciliationResult{}

	// Create multiplicity maps to track position counts per option symbol
	brokerCounts := make(map[string]int)
	localCounts := make(map[string]int)
	localPositionsBySymbol := make(map[string][]models.Position)

	// Build broker position counts by option symbol
	for _, pos := range brokerPositions {
		// Use absolute value since short positions have negative quantities
		// Round to prevent floating-point precision issues (0.999999 -> 0)
		qty := int(math.Round(math.Abs(pos.Quantity)))
		brokerCounts[pos.Symbol] += qty
	}

	// Build local position counts by generating option symbols from strikes/expiration
	for _, pos := range localPositions {
		// Check for credit corruption first
		if pos.CreditReceived <= MinCreditThreshold {
			result.corruptedPositions = append(result.corruptedPositions, pos.ID)
		}

		// Generate option symbols for this position's call and put legs
		callSymbol := b.generateOptionSymbol(pos.Symbol, pos.Expiration, pos.CallStrike, "C")
		putSymbol := b.generateOptionSymbol(pos.Symbol, pos.Expiration, pos.PutStrike, "P")

		// Validate generated symbols
		if callSymbol == "" || putSymbol == "" {
			b.logger.Printf("ERROR: empty OCC symbol for position %s; skipping", pos.ID)
			continue
		}

		// Track each leg separately with normalized quantity
		q := pos.Quantity
		if q < 0 {
			q = -q
		}
		localCounts[callSymbol] += q
		localCounts[putSymbol] += q
		localPositionsBySymbol[callSymbol] = append(localPositionsBySymbol[callSymbol], pos)
		localPositionsBySymbol[putSymbol] = append(localPositionsBySymbol[putSymbol], pos)
	}

	// Get all unique option symbols from both broker and local
	allSymbols := make(map[string]bool)
	for symbol := range brokerCounts {
		allSymbols[symbol] = true
	}
	for symbol := range localCounts {
		allSymbols[symbol] = true
	}

	// Track per-leg excess; we'll resolve it globally after the loop
	phantomOutstanding := make(map[string]int)
	for symbol := range allSymbols {
		brokerCount := brokerCounts[symbol]
		localCount := localCounts[symbol]
		if localCount > brokerCount {
			phantomOutstanding[symbol] = localCount - brokerCount
		}
		if brokerCount > localCount {
			// More broker positions than local positions - mark for recovery
			missingCount := brokerCount - localCount
			for i := 0; i < missingCount; i++ {
				result.brokerOnlyPositions = append(result.brokerOnlyPositions, symbol)
			}
		}
	}

	// Globally choose a minimal set of most-recent positions covering all outstanding legs
	if len(phantomOutstanding) > 0 {
		type posInfo struct {
			qty     int
			entry   time.Time
			symbols map[string]bool
		}
		posByID := make(map[string]*posInfo)
		for sym, positions := range localPositionsBySymbol {
			if phantomOutstanding[sym] <= 0 {
				continue
			}
			for _, p := range positions {
				info := posByID[p.ID]
				if info == nil {
					q := p.Quantity
					if q <= 0 {
						q = 1
					}
					info = &posInfo{qty: q, entry: p.EntryDate, symbols: make(map[string]bool)}
					posByID[p.ID] = info
				}
				info.symbols[sym] = true
			}
		}
		// Sort candidates by most recent first
		ids := make([]string, 0, len(posByID))
		for id := range posByID {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return posByID[ids[i]].entry.After(posByID[ids[j]].entry) })

		// Greedy cover: pick a position only once and subtract its qty from all legs it covers
		for _, id := range ids {
			info := posByID[id]
			helps := false
			for sym := range info.symbols {
				if phantomOutstanding[sym] > 0 {
					helps = true
					break
				}
			}
			if !helps {
				continue
			}
			result.localOnlyPositions = append(result.localOnlyPositions, id)
			for sym := range info.symbols {
				if phantomOutstanding[sym] > 0 {
					phantomOutstanding[sym] -= info.qty
					if phantomOutstanding[sym] < 0 {
						phantomOutstanding[sym] = 0
					}
				}
			}
			done := true
			for _, v := range phantomOutstanding {
				if v > 0 {
					done = false
					break
				}
			}
			if done {
				break
			}
		}

		// Log any leftover deficits for visibility
		for sym, v := range phantomOutstanding {
			if v > 0 {
				b.logger.Printf("METRIC: reconciliation_phantom_leftover=1 symbol=%s deficit=%d", sym, v)
			}
		}
	}

	// Determine if we have any inconsistencies
	result.hasInconsistencies = len(result.brokerOnlyPositions) > 0 ||
		len(result.localOnlyPositions) > 0 ||
		len(result.corruptedPositions) > 0

	return result
}

// logReconciliationIssues provides detailed, actionable guidance for position inconsistencies
func (b *Bot) logReconciliationIssues(result reconciliationResult) {
	b.logger.Printf("⚠️  POSITION INCONSISTENCIES DETECTED")

	if len(result.brokerOnlyPositions) > 0 {
		b.logger.Printf("📋 Positions in broker but missing locally (%d): %v",
			len(result.brokerOnlyPositions), result.brokerOnlyPositions)
	}

	if len(result.localOnlyPositions) > 0 {
		b.logger.Printf("💾 Positions in local storage but missing from broker (%d): %v",
			len(result.localOnlyPositions), result.localOnlyPositions)
	}

	if len(result.corruptedPositions) > 0 {
		b.logger.Printf("🔧 Corrupted positions with invalid credit <= $%.2f (%d): %v",
			MinCreditThreshold, len(result.corruptedPositions), result.corruptedPositions)
	}

	// Provide actionable guidance without hardcoded commands
	b.logger.Printf("🔍 To investigate: Use the audit utility for detailed analysis")
	b.logger.Printf("🧹 To fix: Use the reset_positions utility to sync with broker reality")
	b.logger.Printf("📚 Documentation: See CLAUDE.md for reconciliation troubleshooting")

	// Add specific guidance based on the type of inconsistency
	totalInconsistencies := len(result.brokerOnlyPositions) + len(result.localOnlyPositions) + len(result.corruptedPositions)
	b.logger.Printf("📊 Summary: %d total inconsistencies require attention", totalInconsistencies)
}

// cleanupPhantomPositions removes local positions that don't exist in the broker
func (b *Bot) cleanupPhantomPositions(phantomPositionIDs []string) error {
	if len(phantomPositionIDs) == 0 {
		return nil
	}

	b.logger.Printf("🧹 Cleaning up %d phantom position(s)...", len(phantomPositionIDs))

	for _, posID := range phantomPositionIDs {
		if err := b.storage.DeletePosition(posID); err != nil {
			b.logger.Printf("⚠️  Failed to delete phantom position %s: %v", posID, err)
			continue
		}
		b.logger.Printf("✅ Deleted phantom position: %s", posID)
	}

	return nil
}

// recoverUntrackedPositions adds broker positions that aren't being tracked locally
func (b *Bot) recoverUntrackedPositions(ctx context.Context, brokerPositions []broker.PositionItem, untrackedSymbols []string) error {
	if len(untrackedSymbols) == 0 {
		return nil
	}

	b.logger.Printf("🔄 Recovering %d untracked position(s)...", len(untrackedSymbols))

	// Track multiplicities of untracked symbols
	untrackedMap := make(map[string]int)
	for _, symbol := range untrackedSymbols {
		untrackedMap[symbol]++
	}

	// Group broker positions by their strangle pairs
	strangleGroups := make(map[string][]broker.PositionItem)

	for _, brokerPos := range brokerPositions {
		if untrackedMap[brokerPos.Symbol] <= 0 {
			continue
		}
		untrackedMap[brokerPos.Symbol]--

		// Parse symbol to extract root and expiration for grouping
		root, expiration, _, _, ok := parseOSI(brokerPos.Symbol)
		if !ok {
			b.logger.Printf("⚠️  Skipping malformed option symbol: %s", brokerPos.Symbol)
			continue
		}

		// Group by root + expiration (e.g., "SPY-251107")
		key := fmt.Sprintf("%s-%s", root, expiration.Format("060102"))
		strangleGroups[key] = append(strangleGroups[key], brokerPos)
	}

	// For each expiration group, pair calls and puts by strike to produce multiple positions
	for groupKey, positions := range strangleGroups {
		var calls, puts []broker.PositionItem
		for _, p := range positions {
			optType, ok := extractOptionType(p.Symbol)
			if !ok {
				b.logger.Printf("⚠️  Skipping position with invalid option type: %s", p.Symbol)
				continue
			}
			switch optType {
			case optionTypeCall:
				calls = append(calls, p)
			case optionTypePut:
				puts = append(puts, p)
			}
		}
		if len(calls) == 0 || len(puts) == 0 {
			b.logger.Printf("⚠️  Incomplete strangle group %s (calls=%d, puts=%d), skipping", groupKey, len(calls), len(puts))
			continue
		}
		// Sort by strike to pair deterministically
		sort.Slice(calls, func(i, j int) bool { return extractStrike(calls[i].Symbol) < extractStrike(calls[j].Symbol) })
		sort.Slice(puts, func(i, j int) bool { return extractStrike(puts[i].Symbol) < extractStrike(puts[j].Symbol) })
		n := len(calls)
		if len(puts) < n {
			n = len(puts)
		}
		for i := 0; i < n; i++ {
			recoveredPos := b.createRecoveredPosition([]broker.PositionItem{calls[i], puts[i]})
			if recoveredPos.CreditReceived <= 0 {
				b.logger.Printf("⚠️  Skipping recovered position %s: non-positive credit %.2f", recoveredPos.ID, recoveredPos.CreditReceived)
				continue
			}
			if err := b.storage.AddPosition(&recoveredPos); err != nil {
				b.logger.Printf("❌ Failed to save recovered position %s: %v", recoveredPos.ID, err)
				continue
			}
			b.logger.Printf("✅ Recovered position: %s (pair %d/%d)", recoveredPos.ID, i+1, n)
		}
		if len(calls) != len(puts) {
			b.logger.Printf("ℹ️  Partial recovery for %s: calls=%d puts=%d leftover_calls=%d leftover_puts=%d",
				groupKey, len(calls), len(puts), len(calls)-n, len(puts)-n)
		}
	}

	return nil
}

// createRecoveredPosition creates a Position from broker positions
func (b *Bot) createRecoveredPosition(brokerPositions []broker.PositionItem) models.Position {
	// Use first position to extract common data
	first := brokerPositions[0]

	// Parse via OSI (variable-length root)
	var baseSymbol string
	var expiration time.Time
	if root, exp, _, _, ok := parseOSI(first.Symbol); ok {
		baseSymbol, expiration = root, exp
	} else {
		b.logger.Printf("⚠️  Failed to parse OSI for %s; falling back to legacy slicing", first.Symbol)
		if len(first.Symbol) >= symbolPrefixLength {
			if t, err := time.Parse("060102", first.Symbol[symbolBaseLength:symbolPrefixLength]); err == nil {
				expiration = t
			} else {
				expiration = time.Now().AddDate(0, 0, 45)
			}
			baseSymbol = first.Symbol[:symbolBaseLength]
		} else {
			expiration = time.Now().AddDate(0, 0, 45)
			baseSymbol = first.Symbol
		}
	}

	// Default values
	var callStrike, putStrike float64
	// Aggregate per-contract credit to scale by recovered qty
	var perContractCredit float64
	// Derive minimal contract quantity across legs in this pair
	quantity := -1

	// Separate call and put strikes and calculate per-contract credit
	for _, brokerPos := range brokerPositions {
		optType, ok := extractOptionType(brokerPos.Symbol)
		if !ok {
			b.logger.Printf("⚠️  Skipping position with invalid option type: %s", brokerPos.Symbol)
			continue
		}
		strike := extractStrike(brokerPos.Symbol)

		if strike <= 0 {
			b.logger.Printf("⚠️  Invalid strike price %.2f for symbol %s", strike, brokerPos.Symbol)
			continue
		}

		if optType == optionTypeCall {
			callStrike = strike
		} else if optType == optionTypePut {
			putStrike = strike
		}

		// Per-contract credit (handle multi-contract legs)
		absQ := int(math.Round(math.Abs(brokerPos.Quantity)))
		if absQ > 0 {
			perContractCredit += (-brokerPos.CostBasis) / float64(absQ)
		}
	}

	// Compute recovered quantity as min(abs(leg qty))
	for _, p := range brokerPositions {
		q := int(math.Round(math.Abs(p.Quantity)))
		if q == 0 {
			continue
		}
		if quantity < 0 || q < quantity {
			quantity = q
		}
	}
	if quantity < 1 {
		quantity = 1
	}

	// Validate strikes were found
	if callStrike <= 0 || putStrike <= 0 {
		b.logger.Printf("⚠️  Invalid strikes recovered: call=%.2f, put=%.2f", callStrike, putStrike)
	}

	// Create position using NewPosition factory
	pos := models.NewPosition(
		fmt.Sprintf("recovered-%d", time.Now().UnixNano()),
		baseSymbol,
		putStrike,
		callStrike,
		expiration,
		quantity,
	)

	// Set state to open and credit received
	pos.State = models.StateOpen
	pos.StateMachine = models.NewStateMachineFromState(models.StateOpen)
	pos.EntryDate = time.Now() // We don't know the actual entry time
	pos.CreditReceived = perContractCredit * float64(quantity)
	pos.Adjustments = make([]models.Adjustment, 0)

	return *pos
}

// generateOptionSymbol creates an OCC option symbol from position data
// Format: SPY251107C00690000 (Symbol + YYMMDD + C/P + Strike*1000)
func (b *Bot) generateOptionSymbol(baseSymbol string, expiration time.Time, strike float64, optType string) string {
	// Normalize inputs
	baseSymbol = strings.ToUpper(strings.TrimSpace(baseSymbol))
	optType = strings.ToUpper(strings.TrimSpace(optType))

	// Validate option type is exactly "C" or "P"
	if optType != "C" && optType != "P" {
		b.logger.Printf("ERROR: Invalid option type '%s', must be 'C' or 'P'", optType)
		return ""
	}

	// Format expiration as YYMMDD
	expirationStr := expiration.Format("060102")

	// Format strike as 8-digit integer (strike * 1000) with proper rounding
	strikeInt := int(math.Round(strike * strikeScaleDivisor))
	strikeStr := fmt.Sprintf("%08d", strikeInt)

	return fmt.Sprintf("%s%s%s%s", baseSymbol, expirationStr, optType, strikeStr)
}

// parseOSI parses an OCC option symbol from the end to support any underlying symbol length
// Format: [ROOT][YYMMDD][C/P][STRIKE8]
// Example: SPY251107C00690000, AAPL251107P00150000
// Returns (root, expiration, optionType, strike, ok)
func parseOSI(symbol string) (string, time.Time, string, float64, bool) {
	// OCC format requires minimum 16 chars: 1+ char root, 6 date, 1 type, 8 strike
	const minOSILength = 16
	if len(symbol) < minOSILength {
		return "", time.Time{}, "", 0, false
	}

	// Parse from the end: last 8 = strike, 9th from end = type, 15th-9th from end = date
	strikeCode := symbol[len(symbol)-8:]
	optType := symbol[len(symbol)-9 : len(symbol)-8]
	dateCode := symbol[len(symbol)-15 : len(symbol)-9]
	root := strings.TrimSpace(symbol[:len(symbol)-15])

	// Parse strike (8-digit integer representing price * 1000)
	strikeInt, err := strconv.ParseInt(strikeCode, 10, 64)
	if err != nil {
		return "", time.Time{}, "", 0, false
	}
	strike := float64(strikeInt) / strikeScaleDivisor

	// Parse expiration date (YYMMDD)
	expiration, err := time.Parse("060102", dateCode)
	if err != nil {
		return "", time.Time{}, "", 0, false
	}

	// Validate option type
	if optType != "C" && optType != "P" {
		return "", time.Time{}, "", 0, false
	}

	return root, expiration, optType, strike, true
}

// extractOptionType extracts 'call' or 'put' from option symbol
// Returns (type, ok) where ok indicates if the type was successfully parsed
func extractOptionType(symbol string) (string, bool) {
	_, _, optType, _, ok := parseOSI(symbol)
	if !ok {
		return "", false
	}
	if optType == "C" {
		return optionTypeCall, true
	}
	return optionTypePut, true
}

// extractStrike extracts strike price from option symbol
// Returns 0.0 if the symbol is invalid or parsing fails
func extractStrike(symbol string) float64 {
	_, _, _, strike, ok := parseOSI(symbol)
	if !ok {
		return 0.0
	}
	return strike
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

