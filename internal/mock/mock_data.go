package mock

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
)

type MockDataProvider struct {
	currentPrice float64
	ivr          float64 // IV rank (percentile)
	midIV        float64 // Actual IV level for pricing
}

// secureFloat64 generates a cryptographically secure random float64 between 0 and 1
func secureFloat64() float64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<53))
	if err != nil {
		// Fallback to a reasonable default if crypto/rand fails
		return 0.5
	}
	return float64(n.Int64()) / (1 << 53)
}

// secureInt63n generates a cryptographically secure random int64 between 0 and n-1
func secureInt63n(n int64) int64 {
	max := big.NewInt(n)
	r, err := rand.Int(rand.Reader, max)
	if err != nil {
		// Fallback to a reasonable default if crypto/rand fails
		return n / 2
	}
	return r.Int64()
}

func NewMockDataProvider() *MockDataProvider {
	return &MockDataProvider{
		currentPrice: 450.0 + secureFloat64()*10, // SPY around 450-460
		ivr:          35.0 + secureFloat64()*20,  // IVR between 35-55 (rank)
		midIV:        12.0 + secureFloat64()*18,  // MidIV between 12-30% (actual volatility)
	}
}

func (m *MockDataProvider) GetQuote(symbol string) (*broker.QuoteItem, error) {
	// Simulate small price movements
	m.currentPrice += (secureFloat64() - 0.5) * 2

	spread := 0.02 // 2 cent spread
	return &broker.QuoteItem{
		Symbol: symbol,
		Last:   m.currentPrice,
		Bid:    m.currentPrice - spread/2,
		Ask:    m.currentPrice + spread/2,
		Volume: secureInt63n(100000000),
	}, nil
}

func (m *MockDataProvider) GetIVR() float64 {
	// Simulate IV rank changes
	m.ivr += (secureFloat64() - 0.5) * 2
	m.ivr = math.Max(10, math.Min(90, m.ivr)) // Keep between 10-90
	return m.ivr
}

func (m *MockDataProvider) GetOptionChain(symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	expDate, err := time.Parse("2006-01-02", expiration)
	if err != nil {
		return nil, fmt.Errorf("invalid expiration format: %w", err)
	}
	dte := int(time.Until(expDate).Hours() / 24)
	if dte < 0 {
		dte = 0 // Clamp to minimum 0 to prevent negative time values
	}

	var options []broker.Option

	// Generate strikes around current price
	strikeInterval := 5.0
	startStrike := math.Floor(m.currentPrice/strikeInterval)*strikeInterval - 50
	endStrike := startStrike + 100

	for strike := startStrike; strike <= endStrike; strike += strikeInterval {
		// Calculate approximate delta based on distance from current price
		distance := math.Abs(strike - m.currentPrice)
		deltaDecay := math.Exp(-distance * 0.02) // Exponential decay

		putDelta := -0.5 * deltaDecay
		if strike > m.currentPrice {
			putDelta = -0.5 * (1 - deltaDecay)
		}

		callDelta := 0.5 * deltaDecay
		if strike < m.currentPrice {
			callDelta = 0.5 * (1 - deltaDecay)
		}

		// Calculate option prices (simplified Black-Scholes approximation)
		timeValue := math.Max(0, float64(dte)/365.0) // Ensure timeValue is never negative
		vol := m.midIV / 100.0                       // Use midIV for actual volatility level
		putPrice := math.Max(0.5, vol*math.Sqrt(timeValue)*m.currentPrice*0.01*math.Abs(putDelta))
		callPrice := math.Max(0.5, vol*math.Sqrt(timeValue)*m.currentPrice*0.01*math.Abs(callDelta))

		// Create put option
		putSymbol := fmt.Sprintf("%s%sP%08d", symbol, expDate.Format("060102"), int(strike*1000))
		putOption := broker.Option{
			Symbol:         putSymbol,
			Description:    fmt.Sprintf("%s %s $%.2f Put", symbol, expDate.Format("Jan 02 2006"), strike),
			Strike:         strike,
			OptionType:     "put",
			ExpirationDate: expiration,
			Bid:            putPrice - 0.05,
			Ask:            putPrice + 0.05,
			Last:           putPrice,
			Volume:         secureInt63n(10000),
			OpenInterest:   secureInt63n(50000),
			Underlying:     symbol,
		}

		// Create call option
		callSymbol := fmt.Sprintf("%s%sC%08d", symbol, expDate.Format("060102"), int(strike*1000))
		callOption := broker.Option{
			Symbol:         callSymbol,
			Description:    fmt.Sprintf("%s %s $%.2f Call", symbol, expDate.Format("Jan 02 2006"), strike),
			Strike:         strike,
			OptionType:     "call",
			ExpirationDate: expiration,
			Bid:            callPrice - 0.05,
			Ask:            callPrice + 0.05,
			Last:           callPrice,
			Volume:         secureInt63n(10000),
			OpenInterest:   secureInt63n(50000),
			Underlying:     symbol,
		}

		// Add Greeks if requested
		if withGreeks {
			putOption.Greeks = &broker.Greeks{
				Delta: putDelta,
				MidIV: m.midIV / 100.0, // Use actual midIV level
				Theta: -0.05 * vol,
				Vega:  0.10 * vol,
			}
			callOption.Greeks = &broker.Greeks{
				Delta: callDelta,
				MidIV: m.midIV / 100.0, // Use actual midIV level
				Theta: -0.05 * vol,
				Vega:  0.10 * vol,
			}
		}

		options = append(options, putOption, callOption)
	}

	return options, nil
}

func (m *MockDataProvider) Find16DeltaStrikes(options []broker.Option) (putStrike, callStrike float64) {
	targetDelta := 0.16

	// Find put strike closest to -16 delta
	bestPutStrike := 0.0
	bestPutDiff := math.MaxFloat64

	// Find call strike closest to 16 delta
	bestCallStrike := 0.0
	bestCallDiff := math.MaxFloat64

	for _, option := range options {
		if option.Greeks == nil {
			continue
		}

		if option.OptionType == "put" {
			putDiff := math.Abs(math.Abs(option.Greeks.Delta) - targetDelta)
			if putDiff < bestPutDiff {
				bestPutDiff = putDiff
				bestPutStrike = option.Strike
			}
		} else if option.OptionType == "call" {
			callDiff := math.Abs(option.Greeks.Delta - targetDelta)
			if callDiff < bestCallDiff {
				bestCallDiff = callDiff
				bestCallStrike = option.Strike
			}
		}
	}

	return bestPutStrike, bestCallStrike
}

func (m *MockDataProvider) CalculateStrangleCredit(
	options []broker.Option,
	putStrike, callStrike float64,
) (float64, error) {
	putCredit := 0.0
	callCredit := 0.0

	for _, option := range options {
		if option.Strike == putStrike && option.OptionType == "put" {
			putCredit = (option.Bid + option.Ask) / 2
		}
		if option.Strike == callStrike && option.OptionType == "call" {
			callCredit = (option.Bid + option.Ask) / 2
		}
	}

	if putCredit == 0 || callCredit == 0 {
		return 0, fmt.Errorf("no matching strikes: put=%.2f call=%.2f", putStrike, callStrike)
	}
	return putCredit + callCredit, nil
}

// Generate sample position for testing
func (m *MockDataProvider) GenerateSamplePosition() map[string]interface{} {
	quote, err := m.GetQuote("SPY")
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("quote error: %v", err)}
	}

	expiration := time.Now().AddDate(0, 0, 45).Format("2006-01-02")
	options, err := m.GetOptionChain("SPY", expiration, true)
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("chain error: %v", err)}
	}

	putStrike, callStrike := m.Find16DeltaStrikes(options)
	credit, err := m.CalculateStrangleCredit(options, putStrike, callStrike)
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("credit calculation error: %v", err)}
	}

	expTime, err := time.Parse("2006-01-02", expiration)
	if err != nil {
		// If parse fails, set default expiration 45 days from now
		expTime = time.Now().AddDate(0, 0, 45)
	}
	dte := int(time.Until(expTime).Hours() / 24)
	// Clamp DTE to non-negative value
	if dte < 0 {
		dte = 0
	}

	return map[string]interface{}{
		"symbol":      "SPY",
		"spot_price":  quote.Last,
		"ivr":         m.GetIVR(),
		"put_strike":  putStrike,
		"call_strike": callStrike,
		"credit":      credit,
		"expiration":  expiration,
		"dte":         dte,
	}
}
