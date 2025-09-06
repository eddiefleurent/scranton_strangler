// Package strategy implements trading strategies for options.
package strategy

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
)

// StrangleStrategy implements a short strangle options strategy.
type StrangleStrategy struct {
	broker broker.Broker
	config *StrategyConfig
}

// StrategyConfig contains configuration parameters for the strangle strategy.
type StrategyConfig struct {
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
	UseMockHistoricalIV bool    // Whether to use mock historical IV data
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
func NewStrangleStrategy(b broker.Broker, config *StrategyConfig) *StrangleStrategy {
	return &StrangleStrategy{
		broker: b,
		config: config,
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

	// Get option chain with Greeks
	options, err := s.broker.GetOptionChain(s.config.Symbol, targetExp, true)
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

	// Calculate expected credit
	credit := s.calculateExpectedCredit(options, putStrike, callStrike)

	if credit <= 0 {
		return nil, fmt.Errorf("expected credit is non-positive: %.2f - aborting to prevent loss-making trade", credit)
	}

	if credit < s.config.MinCredit {
		return nil, fmt.Errorf("credit too low: %.2f < %.2f", credit, s.config.MinCredit)
	}

	return &StrangleOrder{
		Symbol:       s.config.Symbol,
		PutStrike:    putStrike,
		CallStrike:   callStrike,
		Expiration:   targetExp,
		Credit:       credit,
		Quantity:     s.calculatePositionSize(credit),
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
		log.Printf("Warning: Using 0.0 IVR due to error: %v", err)
		return 0.0
	}

	// Get historical IV data (20-day lookback for MVP)
	historicalIVs, err := s.getHistoricalImpliedVolatility(20)
	if err != nil || len(historicalIVs) == 0 {
		// Fallback to 0.0 for fail-closed behavior - block entries on insufficient data
		log.Printf("Warning: Using 0.0 IVR due to insufficient historical data")
		return 0.0
	}

	// Calculate IVR using the standard formula
	ivr := broker.CalculateIVR(currentIV, historicalIVs)

	// Log IV calculation details
	log.Printf("IV Rank Calculation: Current IV=%.2f%%, Historical Range=[%.2f%%-%.2f%%], IVR=%.1f",
		currentIV*100, getMinIV(historicalIVs)*100, getMaxIV(historicalIVs)*100, ivr)

	return ivr
}

// getCurrentImpliedVolatility gets current ATM implied volatility for SPY
func (s *StrangleStrategy) getCurrentImpliedVolatility() (float64, error) {
	// Use target expiration (around 45 DTE)
	targetExp := s.findTargetExpiration(s.config.DTETarget)

	// Get option chain for target expiration with Greeks
	chain, err := s.broker.GetOptionChain(s.config.Symbol, targetExp, true)
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
	// Try call then put at ATM
	for _, typ := range []string{"call", "put"} {
		for _, option := range chain {
			if option.Strike == atmStrike && option.OptionType == typ && option.Greeks != nil && option.Greeks.MidIV > 0 {
				return option.Greeks.MidIV, nil
			}
		}
	}
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
		return bestIV, nil
	}

	return 0, fmt.Errorf("no valid IV found for ATM option")
}

// getHistoricalImpliedVolatility retrieves historical IV data
// For MVP, this is a simplified implementation
func (s *StrangleStrategy) getHistoricalImpliedVolatility(days int) ([]float64, error) {
	// TODO: Implement proper historical IV storage/retrieval
	// For MVP, we'll simulate historical data or use a simple approach

	// Option 1: Use mock historical data for testing
	if s.shouldUseMockHistoricalData() {
		return s.generateMockHistoricalIV(days), nil
	}

	// Option 2: Calculate rolling IV from daily option chain snapshots
	// This would require storing daily IV readings
	// For now, return empty to trigger fallback
	return nil, fmt.Errorf("historical IV data not available - implement storage layer")
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

// CalculatePositionPnL calculates current P&L for a position using live option quotes
func (s *StrangleStrategy) CalculatePositionPnL(position *models.Position) (float64, error) {
	if position == nil {
		return 0, fmt.Errorf("position is nil")
	}

	// Get option chain for the position's expiration
	expiration := position.Expiration.Format("2006-01-02")
	chain, err := s.broker.GetOptionChain(position.Symbol, expiration, false)
	if err != nil {
		return 0, fmt.Errorf("failed to get option chain: %w", err)
	}

	// Find current put and call values
	putOption := broker.GetOptionByStrike(chain, position.PutStrike, "put")
	callOption := broker.GetOptionByStrike(chain, position.CallStrike, "call")

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

	// Get option chain for the position's expiration
	expiration := position.Expiration.Format("2006-01-02")
	chain, err := s.broker.GetOptionChain(position.Symbol, expiration, false)
	if err != nil {
		return 0, fmt.Errorf("failed to get option chain: %w", err)
	}

	// Find current put and call values
	putOption := broker.GetOptionByStrike(chain, position.PutStrike, "put")
	callOption := broker.GetOptionByStrike(chain, position.CallStrike, "call")

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
	target := time.Now().AddDate(0, 0, targetDTE)

	// SPY options expire on M/W/F - find the closest expiration
	for {
		weekday := target.Weekday()
		if weekday == time.Monday || weekday == time.Wednesday || weekday == time.Friday {
			break
		}
		target = target.AddDate(0, 0, 1)
	}
	return target.Format("2006-01-02")
}

func (s *StrangleStrategy) findStrikeByDelta(options []broker.Option, targetDelta float64, isPut bool) float64 {
	// Find strike closest to target delta
	bestStrike := 0.0
	bestDiff := math.MaxFloat64

	for _, option := range options {
		// Only consider options of the correct type
		if (isPut && option.OptionType != "put") || (!isPut && option.OptionType != "call") {
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
	put := broker.GetOptionByStrike(options, putStrike, "put")
	call := broker.GetOptionByStrike(options, callStrike, "call")
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
		return 0
	}
	allocatedCapital := balance * s.config.AllocationPct

	// Estimate buying power requirement (simplified)
	// Real calculation would use margin requirements
	bprPerContract := creditPerShare * 100 * 10 // Rough estimate

	maxContracts := int(allocatedCapital / bprPerContract)
	if maxContracts < 1 {
		return 0
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
		log.Printf("Warning: Using stored P&L due to calculation error: %v", err)
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
