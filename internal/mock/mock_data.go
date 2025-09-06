package mock

import (
	"math"
	"math/rand"
	"time"
	
	"github.com/eddie/spy-strangle-bot/internal/broker"
)

type MockDataProvider struct {
	currentPrice float64
	ivr          float64
}

func NewMockDataProvider() *MockDataProvider {
	return &MockDataProvider{
		currentPrice: 450.0 + rand.Float64()*10, // SPY around 450-460
		ivr:          35.0 + rand.Float64()*20,  // IVR between 35-55
	}
}

func (m *MockDataProvider) GetQuote(symbol string) *broker.Quote {
	// Simulate small price movements
	m.currentPrice += (rand.Float64() - 0.5) * 2
	
	spread := 0.02 // 2 cent spread
	return &broker.Quote{
		Symbol: symbol,
		Last:   m.currentPrice,
		Bid:    m.currentPrice - spread/2,
		Ask:    m.currentPrice + spread/2,
		Volume: rand.Int63n(100000000),
	}
}

func (m *MockDataProvider) GetIVR() float64 {
	// Simulate IV rank changes
	m.ivr += (rand.Float64() - 0.5) * 2
	m.ivr = math.Max(10, math.Min(90, m.ivr)) // Keep between 10-90
	return m.ivr
}

func (m *MockDataProvider) GetOptionChain(symbol string, dte int) *broker.OptionChain {
	expiration := time.Now().AddDate(0, 0, dte)
	
	// Find the next Friday
	for expiration.Weekday() != time.Friday {
		expiration = expiration.AddDate(0, 0, 1)
	}
	
	chain := &broker.OptionChain{
		Symbol:     symbol,
		Expiration: expiration.Format("2006-01-02"),
		Strikes:    []broker.OptionStrike{},
	}
	
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
		timeValue := float64(dte) / 365.0
		vol := m.ivr / 100.0
		putPrice := math.Max(0.5, vol*math.Sqrt(timeValue)*m.currentPrice*0.01*math.Abs(putDelta))
		callPrice := math.Max(0.5, vol*math.Sqrt(timeValue)*m.currentPrice*0.01*math.Abs(callDelta))
		
		chain.Strikes = append(chain.Strikes, broker.OptionStrike{
			Strike:     strike,
			CallDelta:  callDelta,
			PutDelta:   putDelta,
			CallBid:    callPrice - 0.05,
			CallAsk:    callPrice + 0.05,
			PutBid:     putPrice - 0.05,
			PutAsk:     putPrice + 0.05,
			CallVolume: rand.Int63n(10000),
			PutVolume:  rand.Int63n(10000),
		})
	}
	
	return chain
}

func (m *MockDataProvider) Find16DeltaStrikes(chain *broker.OptionChain) (putStrike, callStrike float64) {
	targetDelta := 0.16
	
	// Find put strike closest to -16 delta
	bestPutStrike := 0.0
	bestPutDiff := math.MaxFloat64
	
	// Find call strike closest to 16 delta
	bestCallStrike := 0.0
	bestCallDiff := math.MaxFloat64
	
	for _, strike := range chain.Strikes {
		putDiff := math.Abs(math.Abs(strike.PutDelta) - targetDelta)
		if putDiff < bestPutDiff {
			bestPutDiff = putDiff
			bestPutStrike = strike.Strike
		}
		
		callDiff := math.Abs(strike.CallDelta - targetDelta)
		if callDiff < bestCallDiff {
			bestCallDiff = callDiff
			bestCallStrike = strike.Strike
		}
	}
	
	return bestPutStrike, bestCallStrike
}

func (m *MockDataProvider) CalculateStrangleCredit(chain *broker.OptionChain, putStrike, callStrike float64) float64 {
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

// Generate sample position for testing
func (m *MockDataProvider) GenerateSamplePosition() map[string]interface{} {
	quote := m.GetQuote("SPY")
	chain := m.GetOptionChain("SPY", 45)
	putStrike, callStrike := m.Find16DeltaStrikes(chain)
	credit := m.CalculateStrangleCredit(chain, putStrike, callStrike)
	
	return map[string]interface{}{
		"symbol":      "SPY",
		"spot_price":  quote.Last,
		"ivr":         m.GetIVR(),
		"put_strike":  putStrike,
		"call_strike": callStrike,
		"credit":      credit,
		"expiration":  chain.Expiration,
		"dte":         45,
	}
}