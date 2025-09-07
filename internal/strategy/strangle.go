// Package strategy implements trading strategies for options.
package strategy

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
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
	DeltaTarget         float64 // 0.16 for 16 delta
	ProfitTarget        float64 // 0.50 for 50%
	MaxDTE              int     // 21 days to exit
	AllocationPct       float64 // 0.35 for 35%
	MinIVR              float64 // 30
	MinCredit           float64 // $2.00
	EscalateLossPct     float64 // e.g., 2.0 (200% loss triggers escalation)
	StopLossPct         float64 // e.g., 2.5 (250% loss triggers hard stop)
	MaxPositionLoss     float64 // Maximum position loss percentage from risk config
	MaxContracts        int     // Maximum number of contracts per position
	UseMockHistoricalIV bool    // Whether to use mock historical IV data
	FailOpenOnIVError   bool    // If true, allow entries when IV data is unavailable (dev/test only)
	BPRMultiplier       float64 // Buying power requirement multiplier (default: 10.0)
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

	// Check IVR (simplified - would need historical IV data)
	ivr := s.calculateIVR()
	if ivr < s.config.MinIVR {
		return false, fmt.Sprintf("IVR too low: %.1f < %.1f", ivr, s.config.MinIVR)
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

	// Find expiration around 45 DTE
	targetExp := s.findTargetExpiration(s.config.DTETarget)

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

// GetCurrentIVR returns the current IV rank for the configured symbol
func (s *StrangleStrategy) GetCurrentIVR() float64 {
	return s.calculateIVR()
}

func (s *StrangleStrategy) calculateIVR() float64 {
	// Get current IV from option chain
	currentIV, err := s.getCurrentImpliedVolatility()
	if err != nil {
		// Fallback to 0.0 for fail-closed behavior - block entries on IV errors
		s.logger.Printf("Warning: Using 0.0 IVR due to error: %v", err)
		return 0.0
	}

	// Get historical IV data (20-day lookback for MVP)
	historicalIVs, err := s.getHistoricalImpliedVolatility(20)
	if err != nil || len(historicalIVs) == 0 {
		if s.config.FailOpenOnIVError {
			// Fail-open: Use mock data for dev/test when historical data unavailable
			s.logger.Printf("Warning: Using mock historical IV data for IVR calculation (fail-open mode)")
			historicalIVs = s.generateMockHistoricalIV(20)
		} else {
			// Fail-closed: Block entries when historical data unavailable (conservative approach)
			s.logger.Printf("Warning: Using 0.0 IVR due to insufficient historical data (fail-closed mode)")
			return 0.0
		}
	}

	// Calculate IVR using the standard formula
	ivr := broker.CalculateIVR(currentIV, historicalIVs)

	// Log IV calculation details
	s.logger.Printf("IV Rank Calculation: Current IV=%.2f%%, Historical Range=[%.2f%%-%.2f%%], IVR=%.1f%%",
		currentIV*100, getMinIV(historicalIVs)*100, getMaxIV(historicalIVs)*100, ivr)

	return ivr
}

// getCurrentImpliedVolatility gets current ATM implied volatility for SPY
func (s *StrangleStrategy) getCurrentImpliedVolatility() (float64, error) {
	// Use target expiration (around 45 DTE)
	targetExp := s.findTargetExpiration(s.config.DTETarget)

	// Get option chain for target expiration with Greeks (cached)
	chain, err := s.getCachedOptionChain(s.config.Symbol, targetExp, true)
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
	s.storeCurrentIVReading(currentIV, targetExp)

	return currentIV, nil
}

// storeCurrentIVReading stores the current IV reading for future historical analysis
func (s *StrangleStrategy) storeCurrentIVReading(iv float64, expiration string) {
	if s.storage == nil {
		return // No storage available
	}

	// Parse expiration date to validate format
	_, err := time.Parse("2006-01-02", expiration)
	if err != nil {
		s.logger.Printf("Warning: Could not parse expiration date %s for IV storage: %v", expiration, err)
		return
	}

	// Create IV reading for today's date
	now := time.Now().UTC()
	reading := &models.IVReading{
		Symbol:    s.config.Symbol,
		Date:      now.Truncate(24 * time.Hour),
		IV:        iv,
		Timestamp: now,
	}

	// Store the reading
	if err := s.storage.StoreIVReading(reading); err != nil {
		s.logger.Printf("Warning: Failed to store IV reading: %v", err)
	} else {
		s.logger.Printf("Stored IV reading: %s %.2f%% for %s", s.config.Symbol, iv*100, reading.Date.Format("2006-01-02"))
	}
}

// getHistoricalImpliedVolatility retrieves historical IV data
//
// IVR (Implied Volatility Rank) calculation requires historical IV data to compare
// current IV against past levels. When historical data is unavailable:
//
//   - Default behavior (FailOpenOnIVError=false): Returns error, causing IVR=0.0
//     This blocks new position entries as a conservative "fail-closed" approach
//     to prevent trading without proper volatility context.
//
//   - Dev/Test behavior (FailOpenOnIVError=true): Returns mock data to allow
//     testing and development when real historical data is unavailable.
//
// This implementation uses storage-backed IV readings for production use.
func (s *StrangleStrategy) getHistoricalImpliedVolatility(days int) ([]float64, error) {
	// Option 1: Use mock historical data for testing
	if s.shouldUseMockHistoricalData() {
		return s.generateMockHistoricalIV(days), nil
	}

	// Option 2: Retrieve historical IV from storage
	endDate := time.Now().UTC().Truncate(24 * time.Hour)
	startDate := endDate.AddDate(0, 0, -days)

	// Check if storage is available
	if s.storage == nil {
		if s.config.FailOpenOnIVError {
			// Fail-open: Use mock data for dev/test when storage is unset
			s.logger.Printf("Warning: Storage is unset, using mock historical IV data (fail-open mode)")
			return s.generateMockHistoricalIV(days), nil
		} else {
			// Fail-closed: Block entries when storage unavailable (conservative approach)
			s.logger.Printf("Warning: Storage is unset, using 0.0 IVR (fail-closed mode)")
			return nil, fmt.Errorf("storage interface is unset")
		}
	}

	readings, err := s.storage.GetIVReadings(s.config.Symbol, startDate, endDate)
	if err != nil {
		if s.config.FailOpenOnIVError {
			// Fail-open: Use mock data for dev/test when storage fails
			s.logger.Printf("Warning: Storage query failed, using mock historical IV data (fail-open mode): %v", err)
			return s.generateMockHistoricalIV(days), nil
		} else {
			// Fail-closed: Block entries when storage unavailable (conservative approach)
			s.logger.Printf("Warning: Storage query failed, using 0.0 IVR (fail-closed mode): %v", err)
			return nil, fmt.Errorf("historical IV data unavailable from storage: %w", err)
		}
	}

	if len(readings) == 0 {
		if s.config.FailOpenOnIVError {
			// Fail-open: Use mock data when no historical readings exist
			s.logger.Printf("Warning: No historical IV readings found, using mock data (fail-open mode)")
			return s.generateMockHistoricalIV(days), nil
		} else {
			// Fail-closed: Block entries when no historical data exists
			s.logger.Printf("Warning: No historical IV readings found, using 0.0 IVR (fail-closed mode)")
			return nil, fmt.Errorf("no historical IV readings available for symbol %s", s.config.Symbol)
		}
	}

	// Extract IV values (sorted by reading.Date asc for determinism)
	sort.Slice(readings, func(i, j int) bool { return readings[i].Date.Before(readings[j].Date) })
	ivs := make([]float64, len(readings))
	for i, reading := range readings {
		ivs[i] = reading.IV
	}

	return ivs, nil
}

// generateMockHistoricalIV creates realistic mock IV data for testing
func (s *StrangleStrategy) generateMockHistoricalIV(days int) []float64 {
	// Generate realistic IV range for SPY (typically 10-40%)
	historicalIVs := make([]float64, days)

	// Base IV around 20% with some variation
	baseIV := 0.20
	for i := 0; i < days; i++ {
		// Add some realistic variation (±5%)
		variation := (float64(i%10) - 5) * 0.01
		historicalIVs[i] = baseIV + variation

		// Ensure realistic bounds (8% to 35%)
		if historicalIVs[i] < 0.08 {
			historicalIVs[i] = 0.08
		}
		if historicalIVs[i] > 0.35 {
			historicalIVs[i] = 0.35
		}
	}

	return historicalIVs
}

// shouldUseMockHistoricalData determines if we should use mock data
func (s *StrangleStrategy) shouldUseMockHistoricalData() bool {
	// Use config toggle for mock historical IV data
	return s.config.UseMockHistoricalIV
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

// getMinIV finds the minimum IV in historical data
func getMinIV(historicalIVs []float64) float64 {
	if len(historicalIVs) == 0 {
		return 0
	}
	minIV := historicalIVs[0]
	for _, iv := range historicalIVs {
		if iv < minIV {
			minIV = iv
		}
	}
	return minIV
}

// getMaxIV finds the maximum IV in historical data
func getMaxIV(historicalIVs []float64) float64 {
	if len(historicalIVs) == 0 {
		return 0
	}
	maxIV := historicalIVs[0]
	for _, iv := range historicalIVs {
		if iv > maxIV {
			maxIV = iv
		}
	}
	return maxIV
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

// getCachedOptionChain retrieves option chain from cache or fetches from broker if not cached or expired
func (s *StrangleStrategy) getCachedOptionChain(symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	return s.getCachedOptionChainWithFetcher(symbol, expiration, withGreeks, func() ([]broker.Option, error) {
		return s.broker.GetOptionChain(symbol, expiration, withGreeks)
	})
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

	// Get option chain for the position's expiration (cached)
	expiration := position.Expiration.Format("2006-01-02")
	chain, err := s.getCachedOptionChain(position.Symbol, expiration, false)
	if err != nil {
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
	currentTotalValue := (putValue + callValue) * float64(position.Quantity) * 100 // Options are per 100 shares

	// Calculate P&L: Credit received - Current value of sold options
	// (Positive when options lose value, negative when they gain value)
	totalCreditReceived := position.GetTotalCredit() * float64(position.Quantity) * 100
	pnl := totalCreditReceived - currentTotalValue

	return pnl, nil
}

// GetCurrentPositionValue returns the current market value of open options
func (s *StrangleStrategy) GetCurrentPositionValue(position *models.Position) (float64, error) {
	if position == nil {
		return 0, fmt.Errorf("position is nil")
	}

	// Get option chain for the position's expiration (cached)
	expiration := position.Expiration.Format("2006-01-02")
	chain, err := s.getCachedOptionChain(position.Symbol, expiration, false)
	if err != nil {
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

	return (putValue + callValue) * float64(position.Quantity) * 100, nil
}

func (s *StrangleStrategy) hasMajorEventsNearby() bool {
	// Check for FOMC, CPI, etc.
	// For MVP, simplified check
	return false
}

func (s *StrangleStrategy) findTargetExpiration(targetDTE int) string {
	// Create a context with timeout to prevent blocking on slow APIs
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	target := time.Now().AddDate(0, 0, targetDTE)

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

func (s *StrangleStrategy) calculateExpectedCredit(options []broker.Option, putStrike, callStrike float64) float64 {
	put := broker.GetOptionByStrike(options, putStrike, broker.OptionTypePut)
	call := broker.GetOptionByStrike(options, callStrike, broker.OptionTypeCall)
	if put == nil || call == nil {
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

	balance, err := s.broker.GetAccountBalance()
	if err != nil {
		s.logger.Printf("Error getting account balance for sizing: %v", err)
		return 0
	}
	allocatedCapital := balance * s.config.AllocationPct

	// Estimate buying power requirement (simplified)
	// NOTE: Current implementation assumes BPR scales with credit, but for short strangles,
	// margin requirements are more complex and depend on:
	// - Maximum loss potential (span between strikes)
	// - Underlying price
	// - Regulatory minimums (typically 10% of underlying + credit received)
	// TODO: Replace with proper margin calculation based on strike prices and underlying
	bprMultiplier := s.config.BPRMultiplier
	if bprMultiplier <= 0 {
		bprMultiplier = 10.0 // Default to 10x if not configured
	}
	bprPerContract := creditPerShare * 100 * bprMultiplier

	maxContracts := int(allocatedCapital / bprPerContract)
	if maxContracts < 1 {
		return 0
	}

	// Apply config-based MaxContracts cap to keep concerns local
	if s.config.MaxContracts > 0 && maxContracts > s.config.MaxContracts {
		maxContracts = s.config.MaxContracts
		s.logger.Printf("Position size capped at %d contracts (config limit)", maxContracts)
	}

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
	// Calculate profit percentage against total credit received (in dollars)
	totalCredit := position.GetTotalCredit() * float64(position.Quantity) * 100
	if totalCredit == 0 {
		return true, ExitReasonError
	}
	profitPct := currentPnL / totalCredit
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
