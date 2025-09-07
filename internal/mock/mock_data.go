// Package mock provides mock implementations for testing the trading bot components.
package mock

import (
	cryptorand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
)

// DataProvider provides mock market data for testing.
//
// Note: DataProvider isn't goroutine-safe.
// If tests or callers access it concurrently, guard mutable fields with a mutex or document single-threaded use.
type DataProvider struct {
	currentPrice  float64
	ivr           float64    // IV rank (percentile)
	midIV         float64    // Actual IV level for pricing
	deterministic bool       // When true, uses deterministic RNG for stable test outputs
	rng           *rand.Rand // Optional deterministic RNG source
}

// secureFloat64 generates a cryptographically secure random float64 between 0 and 1
func secureFloat64() float64 {
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(1<<53))
	if err != nil {
		// Fallback to a reasonable default if crypto/rand fails
		return 0.5
	}
	return float64(n.Int64()) / (1 << 53)
}

// secureInt63n generates a cryptographically secure random int64 between 0 and n-1
func secureInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	maxVal := big.NewInt(n)
	r, err := cryptorand.Int(cryptorand.Reader, maxVal)
	if err != nil {
		// Fallback to a reasonable default if crypto/rand fails
		return n / 2
	}
	return r.Int64()
}

// randomFloat64 generates a random float64 between 0 and 1, using deterministic RNG if available
func (m *DataProvider) randomFloat64() float64 {
	if m.deterministic && m.rng != nil {
		return m.rng.Float64()
	}
	return secureFloat64()
}

// randomInt63n generates a random int64 between 0 and n-1, using deterministic RNG if available
func (m *DataProvider) randomInt63n(n int64) int64 {
	if m.deterministic && m.rng != nil {
		if n <= 0 {
			return 0
		}
		// Use Int64N to avoid narrowing when n > math.MaxInt
		return m.rng.Int63n(n)
	}
	return secureInt63n(n)
}

// NewDataProvider creates a new mock data provider instance.
func NewDataProvider() *DataProvider {
	return &DataProvider{
		currentPrice: 450.0 + secureFloat64()*10, // SPY around 450-460
		ivr:          35.0 + secureFloat64()*20,  // IVR between 35-55 (rank)
		midIV:        12.0 + secureFloat64()*18,  // MidIV between 12-30% (actual volatility)
	}
}

// NewDeterministicDataProvider creates a new mock data provider with deterministic RNG for testing.
func NewDeterministicDataProvider(seed int64) *DataProvider {
	rng := rand.New(rand.NewSource(seed)) // #nosec G404,G115 -- deterministic random for test data generation
	return &DataProvider{
		currentPrice:  450.0 + rng.Float64()*10, // SPY around 450-460
		ivr:           35.0 + rng.Float64()*20,  // IVR between 35-55 (rank)
		midIV:         12.0 + rng.Float64()*18,  // MidIV between 12-30% (actual volatility)
		deterministic: true,
		rng:           rng,
	}
}

// GetQuote returns mock quote data for the given symbol.
func (m *DataProvider) GetQuote(symbol string) (*broker.QuoteItem, error) {
	// Simulate small price movements
	m.currentPrice += (m.randomFloat64() - 0.5) * 2

	spread := 0.02 // 2 cent spread
	return &broker.QuoteItem{
		Symbol: symbol,
		Last:   m.currentPrice,
		Bid:    m.currentPrice - spread/2,
		Ask:    m.currentPrice + spread/2,
		Volume: m.randomInt63n(100000000),
	}, nil
}

// GetIVR returns a mock implied volatility rank.
func (m *DataProvider) GetIVR() float64 {
	// Simulate IV rank changes
	m.ivr += (m.randomFloat64() - 0.5) * 2
	m.ivr = math.Max(10, math.Min(90, m.ivr)) // Keep between 10-90
	return m.ivr
}

// GetOptionChain returns mock option chain data.
func (m *DataProvider) GetOptionChain(symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
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
		putSymbol := fmt.Sprintf("%s%sP%08d", symbol, expDate.Format("060102"), int(math.Round(strike*1000)))
		putOption := broker.Option{
			Symbol:         putSymbol,
			Description:    fmt.Sprintf("%s %s $%.2f Put", symbol, expDate.Format("Jan 02 2006"), strike),
			Strike:         strike,
			OptionType:     string(broker.OptionTypePut),
			ExpirationDate: expiration,
			Bid:            putPrice - 0.05,
			Ask:            putPrice + 0.05,
			Last:           putPrice,
			Volume:         m.randomInt63n(10000),
			OpenInterest:   m.randomInt63n(50000),
			Underlying:     symbol,
		}

		// Create call option
		callSymbol := fmt.Sprintf("%s%sC%08d", symbol, expDate.Format("060102"), int(math.Round(strike*1000)))
		callOption := broker.Option{
			Symbol:         callSymbol,
			Description:    fmt.Sprintf("%s %s $%.2f Call", symbol, expDate.Format("Jan 02 2006"), strike),
			Strike:         strike,
			OptionType:     string(broker.OptionTypeCall),
			ExpirationDate: expiration,
			Bid:            callPrice - 0.05,
			Ask:            callPrice + 0.05,
			Last:           callPrice,
			Volume:         m.randomInt63n(10000),
			OpenInterest:   m.randomInt63n(50000),
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

// Find16DeltaStrikes finds put and call strikes closest to 16 delta.
func (m *DataProvider) Find16DeltaStrikes(options []broker.Option) (putStrike, callStrike float64) {
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

		switch option.OptionType {
		case string(broker.OptionTypePut):
			putDiff := math.Abs(math.Abs(option.Greeks.Delta) - targetDelta)
			if putDiff < bestPutDiff {
				bestPutDiff = putDiff
				bestPutStrike = option.Strike
			}
		case string(broker.OptionTypeCall):
			callDiff := math.Abs(option.Greeks.Delta - targetDelta)
			if callDiff < bestCallDiff {
				bestCallDiff = callDiff
				bestCallStrike = option.Strike
			}
		}
	}

	// Fallback when no Greeks are present - choose strikes based on distance to current price
	if bestPutStrike == 0 && bestCallStrike == 0 {
		spot := m.currentPrice
		// Simple 16-delta-ish heuristic: ~1.5–2 std devs; pick 10% OTM as placeholder
		putStrike = spot * 0.9
		callStrike = spot * 1.1
		return putStrike, callStrike
	}

	return bestPutStrike, bestCallStrike
}

// CalculateStrangleCredit calculates the credit received for a strangle.
func (m *DataProvider) CalculateStrangleCredit(
	options []broker.Option,
	putStrike, callStrike float64,
) (float64, error) {
	putCredit := 0.0
	callCredit := 0.0

	// Track found options for early exit optimization
	foundPut, foundCall := false, false

	for _, option := range options {
		if math.Abs(option.Strike-putStrike) <= 1e-4 && option.OptionType == string(broker.OptionTypePut) {
			putCredit = (option.Bid + option.Ask) / 2
			foundPut = true
		}
		if math.Abs(option.Strike-callStrike) <= 1e-4 && option.OptionType == string(broker.OptionTypeCall) {
			callCredit = (option.Bid + option.Ask) / 2
			foundCall = true
		}

		// Early exit once both legs are found
		if foundPut && foundCall {
			break
		}
	}

	if putCredit == 0 || callCredit == 0 {
		return 0, fmt.Errorf("no matching strikes: put=%.2f call=%.2f", putStrike, callStrike)
	}
	return putCredit + callCredit, nil
}

// GenerateSamplePosition creates a sample position for testing.
func (m *DataProvider) GenerateSamplePosition() map[string]interface{} {
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
