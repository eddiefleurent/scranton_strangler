// Package strategy implements trading strategies for options.
package strategy

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
)

// optionChainCacheEntry represents a cached option chain entry
type optionChainCacheEntry struct {
	chain     []broker.Option
	timestamp time.Time
}

// Cache TTL for option chains
const optionChainCacheTTL = 1 * time.Minute

// Number of shares per options contract
const sharesPerContract = 100.0

// StrangleStrategy implements a short strangle options strategy.
type StrangleStrategy struct {
	broker     broker.Broker
	config     *Config
	logger     *log.Logger
	chainCache map[string]*optionChainCacheEntry // Cache for option chains by symbol+expiration
	cacheMutex sync.RWMutex                      // Protects concurrent access to chainCache
	storage    storage.Interface                 // Storage for historical IV data
}

// Config contains configuration parameters for the strangle strategy.
type Config struct {
	Symbol              string
	DTETarget           int     // 45 days
	DTERange            []int   // Acceptable DTE range [min, max]
	DeltaTarget         float64 // 0.16 for 16 delta
	ProfitTarget        float64 // 0.50 for 50%
	MaxDTE              int     // 21 days to exit
	AllocationPct       float64 // 0.35 for 35%
	MinIVPct            float64 // 15.0 for 15% SPY ATM IV threshold
	MinCredit           float64 // $2.00
	EscalateLossPct     float64 // e.g., 2.0 (200% loss triggers escalation)
	StopLossPct         float64 // e.g., 2.5 (250% loss triggers hard stop)
	MaxPositionLoss     float64 // Maximum position loss percentage from risk config
	MaxContracts        int     // Maximum number of contracts per position
	BPRMultiplier       float64 // Buying power requirement multiplier (default: 10.0)
	MinVolume           int64   // Minimum daily volume for liquidity filtering (default: 100, 0 to disable)
	MinOpenInterest     int64   // Minimum open interest for liquidity filtering (default: 1000, 0 to disable)
}

// ExitReason represents the reason for exiting a position
type ExitReason string

const (
	// ExitReasonProfitTarget indicates position closed due to profit target hit
	ExitReasonProfitTarget ExitReason = "profit_target"
	// ExitReasonTime indicates exit due to time decay
	ExitReasonTime ExitReason = "time"
	// ExitReasonEscalate indicates exit due to loss escalation
	ExitReasonEscalate ExitReason = "escalate"
	// ExitReasonStopLoss indicates exit due to stop loss
	ExitReasonStopLoss ExitReason = "stop_loss"
	// ExitReasonManual indicates manual exit
	ExitReasonManual ExitReason = "manual"
	// ExitReasonError indicates exit due to error
	ExitReasonError ExitReason = "error"
	// ExitReasonNone indicates no exit reason
	ExitReasonNone ExitReason = "none"
)

// NewStrangleStrategy creates a new strangle strategy instance.
func NewStrangleStrategy(b broker.Broker, config *Config, logger *log.Logger, storage storage.Interface) *StrangleStrategy {
	if logger == nil {
		logger = log.Default()
	}
	return &StrangleStrategy{
		broker:     b,
		config:     config,
		logger:     logger,
		chainCache: make(map[string]*optionChainCacheEntry),
		storage:    storage,
	}
}

// CheckEntryConditions evaluates whether current market conditions are suitable for entry.
func (s *StrangleStrategy) CheckEntryConditions() (bool, string) {
	// Position existence is enforced by storage layer

	// Check volatility threshold using absolute SPY IV level
	canTrade, reason := s.CheckVolatilityThreshold()
	if !canTrade {
		return false, reason
	}

	// Check for major events (simplified)
	if s.hasMajorEventsNearby() {
		return false, "major event within 48 hours"
	}

	return true, "entry conditions met"
}

// FindStrangleStrikes identifies optimal put and call strikes for the strangle.
func (s *StrangleStrategy) FindStrangleStrikes() (*StrangleOrder, error) {
	// Get current SPY price
	quote, err := s.broker.GetQuote(s.config.Symbol)
	if err != nil {
		return nil, err
	}

	// Vary DTE target to avoid identical positions
	targetDTE := s.getVariedDTETarget()
	
	// Find expiration around target DTE
	targetExp := s.findTargetExpiration(targetDTE)

	// Get option chain with Greeks (cached, with timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	options, err := s.getCachedOptionChainWithContext(ctx, s.config.Symbol, targetExp, true)
	if err != nil {
		return nil, err
	}

	// Find strikes closest to target delta
	putStrike := s.findStrikeByDelta(options, -s.config.DeltaTarget, true)
	callStrike := s.findStrikeByDelta(options, s.config.DeltaTarget, false)

	// Validate strikes
	if putStrike == 0 || callStrike == 0 {
		return nil, fmt.Errorf("no strikes found near target delta")
	}

	// Sanity checks for strike selection
	if err := s.validateStrikeSelection(putStrike, callStrike, quote.Last); err != nil {
		return nil, fmt.Errorf("strike validation failed: %w", err)
	}

	// Calculate expected credit
	credit := s.calculateExpectedCredit(options, putStrike, callStrike)

	if credit <= 0 {
		return nil, fmt.Errorf("expected credit is non-positive: %.2f - aborting to prevent loss-making trade", credit)
	}

	if credit < s.config.MinCredit {
		return nil, fmt.Errorf("credit too low: %.2f < %.2f", credit, s.config.MinCredit)
	}

	quantity := s.calculatePositionSize(credit)
	if quantity <= 0 {
		return nil, fmt.Errorf("calculated position size is invalid: %d - unable to allocate capital for trade", quantity)
	}

	return &StrangleOrder{
		Symbol:       s.config.Symbol,
		PutStrike:    putStrike,
		CallStrike:   callStrike,
		Expiration:   targetExp,
		Credit:       credit,
		Quantity:     quantity,
		SpotPrice:    quote.Last,
		ProfitTarget: s.config.ProfitTarget,
	}, nil
}

// GetCurrentIV returns the current SPY ATM implied volatility as a percentage
func (s *StrangleStrategy) GetCurrentIV() float64 {
	currentIV, err := s.getCurrentImpliedVolatility()
	if err != nil {
		s.logger.Printf("Warning: Could not get current %s IV: %v", s.config.Symbol, err)
		return 0.0
	}
	return currentIV * 100 // Convert to percentage
}

// CheckVolatilityThreshold checks if current SPY IV exceeds the minimum threshold for selling
func (s *StrangleStrategy) CheckVolatilityThreshold() (bool, string) {
	currentIV, err := s.getCurrentImpliedVolatility()
	if err != nil {
		return false, fmt.Sprintf("%s IV unavailable: %v", s.config.Symbol, err)
	}
	
	ivPercent := currentIV * 100
	threshold := s.config.MinIVPct // Configurable threshold from config.yaml
	
	if ivPercent >= threshold {
		return true, fmt.Sprintf("%s IV elevated: %.1f%% >= %.1f%%", s.config.Symbol, ivPercent, threshold)
	}
	
	return false, fmt.Sprintf("%s IV too low: %.1f%% < %.1f%%", s.config.Symbol, ivPercent, threshold)
}

// getCurrentImpliedVolatility gets current ATM implied volatility for SPY
func (s *StrangleStrategy) getCurrentImpliedVolatility() (float64, error) {
	// Use target expiration (around 45 DTE)
	targetExp := s.findTargetExpiration(s.config.DTETarget)

	// Get option chain for target expiration with Greeks (cached, timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	chain, err := s.getCachedOptionChainWithContext(ctx, s.config.Symbol, targetExp, true)
	if err != nil {
		return 0, fmt.Errorf("failed to get option chain: %w", err)
	}

	// Get current SPY price
	quote, err := s.broker.GetQuote(s.config.Symbol)
	if err != nil {
		return 0, fmt.Errorf("failed to get quote: %w", err)
	}

	// Prefer ATM call; fall back to ATM put, then nearest-with-IV
	atmStrike := s.findNearestStrike(chain, quote.Last)
	if atmStrike == 0 {
		return 0, fmt.Errorf("no options available for ATM calculation")
	}

	var currentIV float64

	// Try call then put at ATM
	if opt := broker.GetOptionByStrike(chain, atmStrike, broker.OptionTypeCall); opt != nil &&
		opt.Greeks != nil && opt.Greeks.MidIV > 0 {
		currentIV = opt.Greeks.MidIV
	} else if opt := broker.GetOptionByStrike(chain, atmStrike, broker.OptionTypePut); opt != nil &&
		opt.Greeks != nil && opt.Greeks.MidIV > 0 {
		currentIV = opt.Greeks.MidIV
	} else {
		// Nearest strike with a valid IV
		bestIV := 0.0
		bestDiff := math.MaxFloat64
		for _, opt := range chain {
			if opt.Greeks == nil || opt.Greeks.MidIV <= 0 {
				continue
			}
			d := math.Abs(opt.Strike - quote.Last)
			if d < bestDiff {
				bestDiff, bestIV = d, opt.Greeks.MidIV
			}
		}
		if bestIV > 0 {
			currentIV = bestIV
		} else {
			return 0, fmt.Errorf("no valid IV found for ATM option")
		}
	}

	// Store the current IV reading for historical analysis
	s.storeCurrentIVReading(currentIV)

	return currentIV, nil
}

// storeCurrentIVReading stores the current IV reading for future historical analysis
func (s *StrangleStrategy) storeCurrentIVReading(iv float64) {
	if s.storage == nil {
		return // No storage available
	}

	// Create IV reading for today's date
	utcNow := time.Now().UTC()

	// Load America/New_York location with fallback to UTC
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}

	// Get current time in NY timezone
	nyNow := utcNow.In(loc)

	// Create DST-safe local midnight using time.Date
	localMidnight := time.Date(nyNow.Year(), nyNow.Month(), nyNow.Day(), 0, 0, 0, 0, loc)

	reading := &models.IVReading{
		Symbol:    s.config.Symbol,
		Date:      localMidnight,
		IV:        iv,
		Timestamp: utcNow,
	}

	// Store the reading
	if err := s.storage.StoreIVReading(reading); err != nil {
		s.logger.Printf("Warning: Failed to store IV reading: %v", err)
	} else {
		s.logger.Printf("Stored IV reading: %s %.2f%% for %s", s.config.Symbol, iv*100, reading.Date.Format("2006-01-02"))
	}
}




// findNearestStrike finds the strike closest to the current price
func (s *StrangleStrategy) findNearestStrike(chain []broker.Option, price float64) float64 {
	if len(chain) == 0 {
		return 0
	}

	nearestStrike := chain[0].Strike
	minDiff := math.Abs(price - nearestStrike)

	for _, option := range chain {
		diff := math.Abs(price - option.Strike)
		if diff < minDiff {
			minDiff = diff
			nearestStrike = option.Strike
		}
	}

	return nearestStrike
}


// getCachedOptionChainWithFetcher is a helper function that handles the common caching logic
func (s *StrangleStrategy) getCachedOptionChainWithFetcher(symbol, expiration string, withGreeks bool, fetcher func() ([]broker.Option, error)) ([]broker.Option, error) {
	cacheKey := fmt.Sprintf("%s-%s-%t", symbol, expiration, withGreeks)

	// Check cache first (read lock)
	s.cacheMutex.RLock()
	if entry, exists := s.chainCache[cacheKey]; exists {
		if time.Since(entry.timestamp) < optionChainCacheTTL {
			s.cacheMutex.RUnlock()
			return entry.chain, nil
		}
	}
	s.cacheMutex.RUnlock()

	// Clean up expired entries to prevent unbounded cache growth
	s.cleanupExpiredCacheEntries()

	// Fetch from broker using provided fetcher function
	chain, err := fetcher()
	if err != nil {
		return nil, err
	}

	// Cache the result (write lock)
	now := time.Now()
	s.cacheMutex.Lock()
	s.chainCache[cacheKey] = &optionChainCacheEntry{
		chain:     chain,
		timestamp: now,
	}
	s.cacheMutex.Unlock()

	return chain, nil
}

// cleanupExpiredCacheEntries removes expired entries from the cache to prevent unbounded growth
func (s *StrangleStrategy) cleanupExpiredCacheEntries() {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	// Find expired entries
	for key, entry := range s.chainCache {
		if now.Sub(entry.timestamp) >= optionChainCacheTTL {
			expiredKeys = append(expiredKeys, key)
		}
	}

	// Remove expired entries
	for _, key := range expiredKeys {
		delete(s.chainCache, key)
	}

	// Log cleanup if any entries were removed
	if len(expiredKeys) > 0 {
		s.logger.Printf("Cache cleanup: removed %d expired entries", len(expiredKeys))
	}
}


// getCachedOptionChainWithContext retrieves option chain from cache or fetches from broker with context timeout
func (s *StrangleStrategy) getCachedOptionChainWithContext(ctx context.Context, symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	return s.getCachedOptionChainWithFetcher(symbol, expiration, withGreeks, func() ([]broker.Option, error) {
		return s.broker.GetOptionChainCtx(ctx, symbol, expiration, withGreeks)
	})
}

// CalculatePositionPnL calculates current P&L for a position using live option quotes
func (s *StrangleStrategy) CalculatePositionPnL(position *models.Position) (float64, error) {
	if position == nil {
		return 0, fmt.Errorf("position is nil")
	}

	// Get option chain for the position's expiration (cached, with timeout)
	expiration := position.Expiration.Format("2006-01-02")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chain, err := s.getCachedOptionChainWithContext(ctx, position.Symbol, expiration, false)
	if err != nil {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("option chain request timed out: %w", ctx.Err())
		}
		return 0, fmt.Errorf("failed to get option chain: %w", err)
	}

	// Find current put and call values
	putOption := broker.GetOptionByStrike(chain, position.PutStrike, broker.OptionTypePut)
	callOption := broker.GetOptionByStrike(chain, position.CallStrike, broker.OptionTypeCall)

	if putOption == nil || callOption == nil {
		return 0, fmt.Errorf("could not find options for strikes Put %.0f / Call %.0f",
			position.PutStrike, position.CallStrike)
	}

	// Calculate current option values (mid price)
	putValue := (putOption.Bid + putOption.Ask) / 2
	callValue := (callOption.Bid + callOption.Ask) / 2
	currentTotalValue := (putValue + callValue) * float64(position.Quantity) * sharesPerContract // Options are per 100 shares

	// Calculate P&L: Credit received - Current value of sold options
	// (Positive when options lose value, negative when they gain value)
	// GetNetCredit() returns per-share credit, multiply by quantity and shares per contract (100)
	totalCreditReceived := math.Abs(position.GetNetCredit() * float64(position.Quantity) * sharesPerContract)
	pnl := totalCreditReceived - currentTotalValue

	return pnl, nil
}

// GetCurrentPositionValue returns the current market value of open options
func (s *StrangleStrategy) GetCurrentPositionValue(position *models.Position) (float64, error) {
	if position == nil {
		return 0, fmt.Errorf("position is nil")
	}

	// Get option chain for the position's expiration (cached, with timeout)
	expiration := position.Expiration.Format("2006-01-02")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chain, err := s.getCachedOptionChainWithContext(ctx, position.Symbol, expiration, false)
	if err != nil {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("option chain request timed out: %w", ctx.Err())
		}
		return 0, fmt.Errorf("failed to get option chain: %w", err)
	}

	// Find current put and call values
	putOption := broker.GetOptionByStrike(chain, position.PutStrike, broker.OptionTypePut)
	callOption := broker.GetOptionByStrike(chain, position.CallStrike, broker.OptionTypeCall)

	if putOption == nil || callOption == nil {
		return 0, fmt.Errorf("could not find options for strikes Put %.0f / Call %.0f",
			position.PutStrike, position.CallStrike)
	}

	// Calculate current option values (mid price)
	putValue := (putOption.Bid + putOption.Ask) / 2
	callValue := (callOption.Bid + callOption.Ask) / 2

	return (putValue + callValue) * float64(position.Quantity) * sharesPerContract, nil
}

func (s *StrangleStrategy) hasMajorEventsNearby() bool {
	// Check for FOMC, CPI, etc.
	// For MVP, simplified check
	return false
}

// getVariedDTETarget returns a DTE target that varies based on existing positions
// to avoid opening identical trades
func (s *StrangleStrategy) getVariedDTETarget() int {
	// Get existing positions to check their DTEs
	positions := s.storage.GetCurrentPositions()
	
	// If no positions, use the configured target
	if len(positions) == 0 {
		return s.config.DTETarget
	}
	
	// Collect existing DTEs
	existingDTEs := make(map[int]bool)
	for _, pos := range positions {
		dte := pos.CalculateDTE()
		existingDTEs[dte] = true
	}
	
	// Try different DTE values within the configured range
	// Default range if not configured
	minDTE := 40
	maxDTE := 50
	if len(s.config.DTERange) >= 2 {
		minDTE = s.config.DTERange[0]
		maxDTE = s.config.DTERange[1]
	}
	
	// Try to find a DTE that's not already used
	preferredDTEs := []int{45, 43, 47, 41, 49, 40, 50, 42, 48, 44, 46}
	for _, dte := range preferredDTEs {
		if dte >= minDTE && dte <= maxDTE && !existingDTEs[dte] {
			s.logger.Printf("Using varied DTE target: %d (avoiding existing: %v)", dte, existingDTEs)
			return dte
		}
	}
	
	// If all preferred DTEs are taken, just use the configured target
	s.logger.Printf("All preferred DTEs taken, using default: %d", s.config.DTETarget)
	return s.config.DTETarget
}

func (s *StrangleStrategy) findTargetExpiration(targetDTE int) string {
	// Create a context with timeout to prevent blocking on slow APIs
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	target := time.Now().AddDate(0, 0, targetDTE)

	// Ask broker for supported expirations with timeout to prevent hangs
	type expirationsResult struct {
		exps []string
		err  error
	}
	resultChan := make(chan expirationsResult, 1)

	go func() {
		expirations, brokerErr := s.broker.GetExpirations(s.config.Symbol)
		resultChan <- expirationsResult{exps: expirations, err: brokerErr}
	}()

	var exps []string
	var err error
	select {
	case result := <-resultChan:
		exps = result.exps
		err = result.err
	case <-ctx.Done():
		s.logger.Printf("Warning: GetExpirations timed out: %v", ctx.Err())
		err = ctx.Err()
	}

	if err == nil && len(exps) > 0 {
		best := exps[0]
		bestDiff := math.MaxInt32
		for _, e := range exps {
			if d, parseErr := time.Parse("2006-01-02", e); parseErr == nil {
				diff := broker.AbsDaysBetween(target, d)
				if diff < bestDiff {
					best, bestDiff = e, diff
				}
			}
		}
		// Verify chain availability with timeout
		if _, err := s.getCachedOptionChainWithContext(ctx, s.config.Symbol, best, false); err == nil {
			return best
		}
		s.logger.Printf("Best expiration %s failed chain fetch, falling back to hardcoded M/W/F logic", best)
	} else {
		s.logger.Printf("Failed to get broker expirations (%v), falling back to hardcoded M/W/F logic", err)
	}

	// Fallback to hardcoded M/W/F logic if broker expirations fail
	// SPY options expire on M/W/F - find the closest expiration
	for {
		weekday := target.Weekday()
		if weekday == time.Monday || weekday == time.Wednesday || weekday == time.Friday {
			break
		}
		target = target.AddDate(0, 0, 1)
	}

	// Check if option chain data is available for this date
	// If not (e.g., holiday), iterate forward until we find valid data
	// Bound the search to prevent infinite loops (max 7 days forward)
	const maxIterations = 7
	for i := 0; i < maxIterations; i++ {
		expirationStr := target.Format("2006-01-02")

		// Try to get option chain to verify data availability with timeout
		_, err := s.getCachedOptionChainWithContext(ctx, s.config.Symbol, expirationStr, false)
		if err == nil {
			// Option chain available, use this date
			return expirationStr
		}

		// Check if context was cancelled (timeout)
		if ctx.Err() != nil {
			s.logger.Printf("Context cancelled during expiration probing: %v, using fallback", ctx.Err())
			break
		}

		// Log the attempt and try next valid expiration date
		s.logger.Printf("No option chain available for %s (possibly holiday), trying next expiration", expirationStr)

		// Move to next M/W/F
		for {
			target = target.AddDate(0, 0, 1)
			weekday := target.Weekday()
			if weekday == time.Monday || weekday == time.Wednesday || weekday == time.Friday {
				break
			}
		}
	}

	// If we couldn't find valid data after max iterations, return the original target
	// This is a fallback to avoid breaking the system completely
	originalTarget := time.Now().AddDate(0, 0, targetDTE)
	for {
		weekday := originalTarget.Weekday()
		if weekday == time.Monday || weekday == time.Wednesday || weekday == time.Friday {
			break
		}
		originalTarget = originalTarget.AddDate(0, 0, 1)
	}
	s.logger.Printf("Warning: Could not find valid option chain data, using fallback date")
	return originalTarget.Format("2006-01-02")
}

func (s *StrangleStrategy) findStrikeByDelta(options []broker.Option, targetDelta float64, isPut bool) float64 {
	// Find strike closest to target delta
	bestStrike := 0.0
	bestDiff := math.MaxFloat64

	for _, option := range options {
		// Only consider options of the correct type
		if (isPut && !broker.OptionTypeMatches(option.OptionType, broker.OptionTypePut)) ||
			(!isPut && !broker.OptionTypeMatches(option.OptionType, broker.OptionTypeCall)) {
			continue
		}

		// Skip if no Greeks available
		if option.Greeks == nil {
			continue
		}

		// Optional liquidity filter: skip illiquid options if configured thresholds are set
		if s.shouldFilterForLiquidity(&option) {
			continue
		}

		delta := option.Greeks.Delta
		var diff float64
		if isPut {
			// For puts, compare absolute delta to absolute target delta
			diff = math.Abs(math.Abs(delta) - math.Abs(targetDelta))
		} else {
			// For calls, compare delta directly
			diff = math.Abs(delta - targetDelta)
		}
		if diff < bestDiff {
			bestDiff = diff
			bestStrike = option.Strike
		}
	}

	return bestStrike
}

// shouldFilterForLiquidity determines if an option should be filtered out due to low liquidity
func (s *StrangleStrategy) shouldFilterForLiquidity(option *broker.Option) bool {
	// If both thresholds are 0 or negative, liquidity filtering is disabled
	if s.config.MinVolume <= 0 && s.config.MinOpenInterest <= 0 {
		return false
	}

	// Set defaults if not configured (based on best practices for options liquidity)
	minVolume := s.config.MinVolume
	minOpenInterest := s.config.MinOpenInterest
	if minVolume <= 0 {
		minVolume = 100 // Default minimum volume - good for immediate liquidity
	}
	if minOpenInterest <= 0 {
		minOpenInterest = 1000 // Default minimum open interest - good for sustained liquidity
	}

	// Skip this filter if data is not available (e.g., test scenarios) to avoid filtering out valid options
	if option.Volume == 0 && option.OpenInterest == 0 {
		return false
	}

	// Filter out options that don't meet minimum liquidity requirements
	// Both volume AND open interest must meet thresholds (if data is available)
	hasInsufficientVolume := option.Volume > 0 && option.Volume < minVolume
	hasInsufficientOI := option.OpenInterest > 0 && option.OpenInterest < minOpenInterest

	// Filter if either volume or OI is insufficient (when data is available)
	return hasInsufficientVolume || hasInsufficientOI
}

func (s *StrangleStrategy) calculateExpectedCredit(options []broker.Option, putStrike, callStrike float64) float64 {
	put := broker.GetOptionByStrike(options, putStrike, broker.OptionTypePut)
	call := broker.GetOptionByStrike(options, callStrike, broker.OptionTypeCall)
	if put == nil || call == nil {
		return 0
	}
	// Get tick size from broker for proper spread validation
	var tickSize float64 = 0.01 // Default fallback
	if s.broker != nil {
		if brokerTickSize, err := s.broker.GetTickSize(s.config.Symbol); err == nil && brokerTickSize > 0 {
			tickSize = brokerTickSize
		}
	}

	// Calculate minimum spread as at least one tick (or 0.01 minimum)
	putMinSpread := math.Max(tickSize, 0.01)
	callMinSpread := math.Max(tickSize, 0.01)

	// Check each leg against its own minimum spread to filter stale/microstructure quotes
	if (put.Ask-put.Bid) < putMinSpread || (call.Ask-call.Bid) < callMinSpread {
		return 0
	}
	// Guard against stale/invalid quotes
	if put.Bid <= 0 || put.Ask <= 0 || call.Bid <= 0 || call.Ask <= 0 || put.Bid > put.Ask || call.Bid > call.Ask {
		return 0
	}
	putCredit := (put.Bid + put.Ask) / 2
	callCredit := (call.Bid + call.Ask) / 2
	return putCredit + callCredit
}

func (s *StrangleStrategy) calculatePositionSize(creditPerShare float64) int {
	// Defense-in-depth: guard against zero/negative quantities
	if creditPerShare <= 0 {
		return 0
	}

	// Try to use option buying power first (more accurate for options trading)
	buyingPower, err := s.broker.GetOptionBuyingPower()
	if err != nil {
		s.logger.Printf("Warning: Could not get option buying power, falling back to account balance: %v", err)
		// Fall back to account balance
		balance, err := s.broker.GetAccountBalance()
		if err != nil {
			s.logger.Printf("Error getting account balance for sizing: %v", err)
			return 0
		}
		buyingPower = balance * 0.5 // Conservative estimate: assume 50% of balance is available for options
	}
	
	alloc := s.config.AllocationPct
	if alloc < 0 {
		alloc = 0
	} else if alloc > 1 {
		alloc = 1
	}
	allocatedCapital := buyingPower * alloc

	// Estimate buying power requirement for short strangles
	// For paper trading, use a simplified calculation
	// Typical margin requirement for short strangles: 20% of underlying + credit received
	// But for simplicity in paper trading, we'll use a conservative estimate
	creditTotal := creditPerShare * sharesPerContract
	
	// Estimate margin requirement: typically 15-20% of notional for SPY strangles
	// Using 15% for paper trading to allow more positions
	estimatedMargin := creditTotal * 5.0 // 5x credit as a rough margin estimate
	
	s.logger.Printf("Sizing: credit/contract=$%.2f, est margin/contract=$%.2f, allocated buying power=$%.2f",
		creditTotal, estimatedMargin, allocatedCapital)

	maxContracts := int(allocatedCapital / estimatedMargin)
	if maxContracts < 1 {
		s.logger.Printf("Insufficient buying power for even 1 contract (need $%.2f, have $%.2f allocated)", 
			estimatedMargin, allocatedCapital)
		return 0
	}

	// Apply config-based MaxContracts cap to keep concerns local
	if s.config.MaxContracts > 0 && maxContracts > s.config.MaxContracts {
		maxContracts = s.config.MaxContracts
		s.logger.Printf("Position size capped at %d contracts (config limit)", maxContracts)
	}

	s.logger.Printf("Calculated position size: %d contracts", maxContracts)
	return maxContracts
}

// CheckExitConditions checks if a position should be exited
func (s *StrangleStrategy) CheckExitConditions(position *models.Position) (bool, ExitReason) {
	if position == nil {
		return false, ExitReasonNone
	}

	// Calculate real-time P&L
	currentPnL, err := s.CalculatePositionPnL(position)
	if err != nil {
		// Fall back to stored P&L if real-time calculation fails
		currentPnL = position.CurrentPnL
		s.logger.Printf("Warning: Using stored P&L due to calculation error: %v", err)
	}

	// Check profit target
	// Calculate profit percentage against abs(net credit) received (in dollars)
	// GetNetCredit() returns per-share credit, multiply by quantity and shares per contract
	absTotalNetCredit := math.Abs(position.GetNetCredit()) * float64(position.Quantity) * sharesPerContract
	if absTotalNetCredit == 0 {
		return true, ExitReasonError
	}
	profitPct := currentPnL / absTotalNetCredit
	if profitPct >= s.config.ProfitTarget {
		return true, ExitReasonProfitTarget
	}

	// Check escalate loss threshold - prepare for action
	escalateThreshold := s.config.EscalateLossPct
	if escalateThreshold <= 0 {
		escalateThreshold = 2.0 // Default to 200% loss
	}
	if profitPct <= -escalateThreshold {
		// First check if we've reached the stop loss threshold
		sl := s.config.StopLossPct
		if sl <= 0 {
			sl = 2.5
		} // Default to 250% to match old behavior
		// Clamp the stop-loss to the risk cap only when MaxPositionLoss is explicitly set (> 0)
		if s.config.MaxPositionLoss > 0 && sl > s.config.MaxPositionLoss {
			sl = s.config.MaxPositionLoss
		}
		if profitPct <= -sl {
			return true, ExitReasonStopLoss
		}
		// Otherwise trigger escalate action
		return true, ExitReasonEscalate
	}

	// Check DTE using strategy config
	currentDTE := position.CalculateDTE()
	if currentDTE <= s.config.MaxDTE {
		return true, ExitReasonTime
	}

	return false, ExitReasonNone
}

// CalculatePnL calculates the current profit/loss for a position.
func (s *StrangleStrategy) CalculatePnL(pos *models.Position) float64 {
	// Use the unified CalculatePositionPnL implementation
	pnl, err := s.CalculatePositionPnL(pos)
	if err != nil {
		// Return stored P&L if calculation fails
		return pos.CurrentPnL
	}
	return pnl
}

// validateStrikeSelection performs sanity checks on selected strikes
func (s *StrangleStrategy) validateStrikeSelection(putStrike, callStrike, spotPrice float64) error {
	// Validate spot price before any calculations to prevent division by zero or invalid values
	if spotPrice <= 0 || math.IsNaN(spotPrice) || math.IsInf(spotPrice, 0) {
		return fmt.Errorf("invalid spot price: %v", spotPrice)
	}

	// Check for inverted strikes (put should be below call)
	if putStrike >= callStrike {
		return fmt.Errorf("inverted strikes detected: put %.0f >= call %.0f", putStrike, callStrike)
	}

	// Calculate spread width as percentage of spot price
	spreadWidth := (callStrike - putStrike) / spotPrice

	// Reject excessively tight spreads (< 1% of spot price)
	const minSpreadPct = 0.01
	if spreadWidth < minSpreadPct {
		return fmt.Errorf("spread too tight: %.2f%% < %.2f%% (put=%.0f, call=%.0f)",
			spreadWidth*100, minSpreadPct*100, putStrike, callStrike)
	}

	// Reject excessively wide spreads (> 10% of spot price)
	const maxSpreadPct = 0.10
	if spreadWidth > maxSpreadPct {
		return fmt.Errorf("spread too wide: %.2f%% > %.2f%% (put=%.0f, call=%.0f)",
			spreadWidth*100, maxSpreadPct*100, putStrike, callStrike)
	}

	// Check that strikes are reasonably close to spot price
	// Put should not be more than 15% below spot, call not more than 15% above
	const maxDeviationPct = 0.15
	putDeviation := (spotPrice - putStrike) / spotPrice
	callDeviation := (callStrike - spotPrice) / spotPrice

	if putDeviation > maxDeviationPct {
		return fmt.Errorf("put strike too far from spot: %.0f (%.1f%% below spot)",
			putStrike, putDeviation*100)
	}
	if callDeviation > maxDeviationPct {
		return fmt.Errorf("call strike too far from spot: %.0f (%.1f%% above spot)",
			callStrike, callDeviation*100)
	}

	return nil
}

// StrangleOrder represents the details of a strangle order to be placed.
type StrangleOrder struct {
	Symbol       string
	Expiration   string
	PutStrike    float64
	CallStrike   float64
	Credit       float64
	Quantity     int
	SpotPrice    float64
	ProfitTarget float64
}
