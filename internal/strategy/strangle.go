package strategy

import (
	"fmt"
	"math"
	"time"
	
	"github.com/eddie/spy-strangle-bot/internal/broker"
	"github.com/eddie/spy-strangle-bot/internal/models"
)

type StrangleStrategy struct {
	broker       *broker.TradierClient
	config       *StrategyConfig
	currentPos   *models.Position
}

type StrategyConfig struct {
	Symbol        string
	DTETarget     int     // 45 days
	DeltaTarget   float64 // 0.16 for 16 delta
	ProfitTarget  float64 // 0.50 for 50%
	MaxDTE        int     // 21 days to exit
	AllocationPct float64 // 0.35 for 35%
	MinIVR        float64 // 30
	MinCredit     float64 // $2.00
}

func NewStrangleStrategy(broker *broker.TradierClient, config *StrategyConfig) *StrangleStrategy {
	return &StrangleStrategy{
		broker: broker,
		config: config,
	}
}

func (s *StrangleStrategy) CheckEntryConditions() (bool, string) {
	// Check if we already have a position
	if s.currentPos != nil && s.currentPos.Status == "open" {
		return false, "already have open position"
	}
	
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

func (s *StrangleStrategy) FindStrangleStrikes() (*StrangleOrder, error) {
	// Get current SPY price
	quote, err := s.broker.GetQuote(s.config.Symbol)
	if err != nil {
		return nil, err
	}
	
	// Find expiration around 45 DTE
	targetExp := s.findTargetExpiration(s.config.DTETarget)
	
	// Get option chain
	chain, err := s.broker.GetOptionChain(s.config.Symbol, targetExp)
	if err != nil {
		return nil, err
	}
	
	// Find strikes closest to target delta
	putStrike := s.findStrikeByDelta(chain, -s.config.DeltaTarget, true)
	callStrike := s.findStrikeByDelta(chain, s.config.DeltaTarget, false)
	
	// Calculate expected credit
	credit := s.calculateExpectedCredit(chain, putStrike, callStrike)
	
	if credit < s.config.MinCredit {
		return nil, fmt.Errorf("credit too low: %.2f < %.2f", credit, s.config.MinCredit)
	}
	
	return &StrangleOrder{
		Symbol:     s.config.Symbol,
		PutStrike:  putStrike,
		CallStrike: callStrike,
		Expiration: targetExp,
		Credit:     credit,
		Quantity:   s.calculatePositionSize(credit),
		SpotPrice:  quote.Last,
	}, nil
}

func (s *StrangleStrategy) calculateIVR() float64 {
	// Simplified IVR calculation
	// In reality, would need 52-week IV history
	// For MVP, using a mock calculation
	return 35.0 // Mock value above threshold
}

func (s *StrangleStrategy) hasMajorEventsNearby() bool {
	// Check for FOMC, CPI, etc.
	// For MVP, simplified check
	return false
}

func (s *StrangleStrategy) findTargetExpiration(targetDTE int) string {
	target := time.Now().AddDate(0, 0, targetDTE)
	// Find the Friday closest to target
	for target.Weekday() != time.Friday {
		target = target.AddDate(0, 0, 1)
	}
	return target.Format("2006-01-02")
}

func (s *StrangleStrategy) findStrikeByDelta(chain *broker.OptionChain, targetDelta float64, isPut bool) float64 {
	// Find strike closest to target delta
	bestStrike := 0.0
	bestDiff := math.MaxFloat64
	
	for _, strike := range chain.Strikes {
		var delta float64
		if isPut {
			delta = strike.PutDelta
		} else {
			delta = strike.CallDelta
		}
		
		diff := math.Abs(delta - targetDelta)
		if diff < bestDiff {
			bestDiff = diff
			bestStrike = strike.Strike
		}
	}
	
	return bestStrike
}

func (s *StrangleStrategy) calculateExpectedCredit(chain *broker.OptionChain, putStrike, callStrike float64) float64 {
	putCredit := 0.0
	callCredit := 0.0
	
	for _, strike := range chain.Strikes {
		if strike.Strike == putStrike {
			putCredit = (strike.PutBid + strike.PutAsk) / 2
		}
		if strike.Strike == callStrike {
			callCredit = (strike.CallBid + strike.CallAsk) / 2
		}
	}
	
	return putCredit + callCredit
}

func (s *StrangleStrategy) calculatePositionSize(creditPerContract float64) int {
	balance, _ := s.broker.GetAccountBalance()
	allocatedCapital := balance * s.config.AllocationPct
	
	// Estimate buying power requirement (simplified)
	// Real calculation would use margin requirements
	bprPerContract := creditPerContract * 100 * 10 // Rough estimate
	
	maxContracts := int(allocatedCapital / bprPerContract)
	if maxContracts < 1 {
		maxContracts = 1
	}
	
	return maxContracts
}

func (s *StrangleStrategy) CheckExitConditions() (bool, string) {
	if s.currentPos == nil {
		return false, "no position"
	}
	
	// Check profit target
	profitPct := s.currentPos.CurrentPnL / s.currentPos.CreditReceived
	if profitPct >= s.config.ProfitTarget {
		return true, fmt.Sprintf("profit target reached: %.1f%%", profitPct*100)
	}
	
	// Check DTE
	if s.currentPos.DTE <= s.config.MaxDTE {
		return true, fmt.Sprintf("max DTE reached: %d days", s.currentPos.DTE)
	}
	
	return false, "no exit conditions met"
}

func (s *StrangleStrategy) CalculatePnL(pos *models.Position) float64 {
	// Get current option prices
	quote, _ := s.broker.GetQuote(s.config.Symbol)
	chain, _ := s.broker.GetOptionChain(s.config.Symbol, pos.Expiration.Format("2006-01-02"))
	
	currentCost := 0.0
	for _, strike := range chain.Strikes {
		if strike.Strike == pos.PutStrike {
			currentCost += (strike.PutBid + strike.PutAsk) / 2
		}
		if strike.Strike == pos.CallStrike {
			currentCost += (strike.CallBid + strike.CallAsk) / 2
		}
	}
	
	// P&L = credit received - current cost to close
	return pos.CreditReceived - currentCost
}

type StrangleOrder struct {
	Symbol     string
	PutStrike  float64
	CallStrike float64
	Expiration string
	Credit     float64
	Quantity   int
	SpotPrice  float64
}