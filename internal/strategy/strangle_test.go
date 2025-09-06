package strategy

import (
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
)


func TestStrangleStrategy_calculatePositionSize(t *testing.T) {
	tests := []struct {
		name                 string
		accountBalance       float64
		allocationPct        float64
		creditPerContract    float64
		expectedMinContracts int
	}{
		{
			name:                 "normal account size",
			accountBalance:       100000.0, // $100k account
			allocationPct:        0.35,     // 35% allocation
			creditPerContract:    3.50,     // $3.50 credit
			expectedMinContracts: 1,        // At least 1 contract
		},
		{
			name:                 "small account",
			accountBalance:       10000.0, // $10k account
			allocationPct:        0.35,    // 35% allocation
			creditPerContract:    2.00,    // $2.00 credit
			expectedMinContracts: 1,       // Minimum 1 contract
		},
		{
			name:                 "large account high allocation",
			accountBalance:       500000.0, // $500k account
			allocationPct:        0.35,     // 35% allocation
			creditPerContract:    4.00,     // $4.00 credit
			expectedMinContracts: 1,        // Should allow multiple contracts
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock broker that returns the test balance
			mockClient := newMockBroker(tt.accountBalance)

			config := &StrategyConfig{
				AllocationPct: tt.allocationPct,
			}

			strategy := &StrangleStrategy{
				broker: mockClient,
				config: config,
			}

			result := strategy.calculatePositionSize(tt.creditPerContract)

			if result < tt.expectedMinContracts {
				t.Errorf("calculatePositionSize() = %v, want at least %v",
					result, tt.expectedMinContracts)
			}

			// Verify allocation doesn't exceed limit
			allocatedCapital := tt.accountBalance * tt.allocationPct
			estimatedBPR := tt.creditPerContract * 100 * 10 * float64(result)
			if estimatedBPR > allocatedCapital*1.1 { // 10% tolerance
				t.Errorf("Position size %v exceeds allocation: BPR=%.2f, Allocated=%.2f",
					result, estimatedBPR, allocatedCapital)
			}
		})
	}
}

func TestStrangleStrategy_calculateExpectedCredit(t *testing.T) {
	options := []broker.Option{
		{
			Strike:     400.0,
			OptionType: "put",
			Bid:        1.50,
			Ask:        1.60,
		},
		{
			Strike:     400.0,
			OptionType: "call",
			Bid:        0.80,
			Ask:        0.90,
		},
		{
			Strike:     420.0,
			OptionType: "put",
			Bid:        3.00,
			Ask:        3.20,
		},
		{
			Strike:     420.0,
			OptionType: "call",
			Bid:        2.10,
			Ask:        2.30,
		},
	}

	strategy := &StrangleStrategy{}

	tests := []struct {
		name       string
		putStrike  float64
		callStrike float64
		expected   float64
	}{
		{
			name:       "normal strangle",
			putStrike:  400.0,
			callStrike: 420.0,
			expected:   3.75, // (1.55 + 2.20) = put mid + call mid
		},
		{
			name:       "same strike (straddle)",
			putStrike:  400.0,
			callStrike: 400.0,
			expected:   2.40, // (1.55 + 0.85) = both from 400 strike
		},
		{
			name:       "non-existent strikes",
			putStrike:  350.0,
			callStrike: 450.0,
			expected:   0.0, // No matching strikes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.calculateExpectedCredit(options, tt.putStrike, tt.callStrike)
			tolerance := 0.01
			if result < tt.expected-tolerance || result > tt.expected+tolerance {
				t.Errorf("calculateExpectedCredit() = %.2f, want %.2f (±%.2f)", result, tt.expected, tolerance)
			}
		})
	}
}

func TestStrangleStrategy_CheckExitConditions(t *testing.T) {
	config := &StrategyConfig{
		Symbol:       "SPY",
		ProfitTarget: 0.50, // 50%
		MaxDTE:       21,   // 21 days
	}

	tests := []struct {
		position       *models.Position
		name           string
		expectedReason string
		expectedExit   bool
	}{
		{
			name:           "no position",
			position:       nil,
			expectedExit:   false,
			expectedReason: "",
		},
		{
			name: "profit target reached",
			position: &models.Position{
				Symbol:         "SPY",
				PutStrike:      400.0,
				CallStrike:     420.0,
				Expiration:     time.Now().AddDate(0, 0, 35),
				Quantity:       1,
				CreditReceived: 3.50,
				CurrentPnL:     175.0, // 50% profit ($175 on $350 credit)
				DTE:            35,
			},
			expectedExit:   true,
			expectedReason: "profit_target",
		},
		{
			name: "max DTE reached",
			position: &models.Position{
				Symbol:         "SPY",
				PutStrike:      400.0,
				CallStrike:     420.0,
				Expiration:     time.Now().AddDate(0, 0, 21),
				Quantity:       1,
				CreditReceived: 3.50,
				CurrentPnL:     50.0, // Only 14% profit ($50 on $350 credit)
				DTE:            21,   // At max DTE
			},
			expectedExit:   true,
			expectedReason: "time",
		},
		{
			name: "no exit conditions met",
			position: &models.Position{
				Symbol:         "SPY",
				PutStrike:      400.0,
				CallStrike:     420.0,
				Expiration:     time.Now().AddDate(0, 0, 35),
				Quantity:       1,
				CreditReceived: 3.50,
				CurrentPnL:     50.0, // Only 14% profit ($50 on $350 credit)
				DTE:            35,   // Still have time
			},
			expectedExit:   false,
			expectedReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockBroker(100000.0)
			strategy := &StrangleStrategy{
				broker: mockClient,
				config: config,
			}

			// Set up mock option prices based on test case
			if tt.position != nil {
				expiration := tt.position.Expiration.Format("2006-01-02")

				// Set different option prices for different test scenarios
				switch tt.name {
				case "profit target reached":
					// For 50% profit: credit 3.50, current value should be 1.75
					mockClient.setOptionPrice(expiration, 400.0, "put", 0.85)
					mockClient.setOptionPrice(expiration, 420.0, "call", 0.90)
					// Total: 1.75, P&L = 3.50 - 1.75 = 1.75 (50% profit)
				case "max DTE reached":
					// Current value higher, low profit but time exit
					// Want 0.50 P&L: Credit $350, current value should be $300
					mockClient.setOptionPrice(expiration, 400.0, "put", 1.50)
					mockClient.setOptionPrice(expiration, 420.0, "call", 1.50)
					// Total: 3.00, P&L = ($350 - $300) = $50 (14% profit)
				case "no exit conditions met":
					// Same as max DTE but different DTE in position
					mockClient.setOptionPrice(expiration, 400.0, "put", 1.50)
					mockClient.setOptionPrice(expiration, 420.0, "call", 1.50)
					// Total: 3.00, P&L = ($350 - $300) = $50 (14% profit)
				}
			}

			strategy.currentPos = tt.position

			shouldExit, reason := strategy.CheckExitConditions(tt.position)

			if shouldExit != tt.expectedExit {
				t.Errorf("CheckExitConditions() exit = %v, want %v", shouldExit, tt.expectedExit)
			}

			if !containsSubstring(string(reason), tt.expectedReason) {
				t.Errorf("CheckExitConditions() reason = %q, want to contain %q", reason, tt.expectedReason)
			}
		})
	}
}

func TestStrangleStrategy_findStrikeByDelta(t *testing.T) {
	options := []broker.Option{
		{
			Strike:     400.0,
			OptionType: "put",
			Greeks:     &broker.Greeks{Delta: -0.20},
		},
		{
			Strike:     400.0,
			OptionType: "call",
			Greeks:     &broker.Greeks{Delta: 0.25},
		},
		{
			Strike:     410.0,
			OptionType: "put",
			Greeks:     &broker.Greeks{Delta: -0.16},
		},
		{
			Strike:     410.0,
			OptionType: "call",
			Greeks:     &broker.Greeks{Delta: 0.18},
		},
		{
			Strike:     420.0,
			OptionType: "put",
			Greeks:     &broker.Greeks{Delta: -0.12},
		},
		{
			Strike:     420.0,
			OptionType: "call",
			Greeks:     &broker.Greeks{Delta: 0.14},
		},
	}

	strategy := &StrangleStrategy{}

	tests := []struct {
		name        string
		targetDelta float64
		isPut       bool
		expected    float64
	}{
		{
			name:        "find 16 delta put",
			targetDelta: -0.16,
			isPut:       true,
			expected:    410.0, // Exact match
		},
		{
			name:        "find 16 delta call",
			targetDelta: 0.16,
			isPut:       false,
			expected:    410.0, // Closest to 0.18
		},
		{
			name:        "find closest put when no exact match",
			targetDelta: -0.15,
			isPut:       true,
			expected:    410.0, // -0.16 is closest to -0.15
		},
		{
			name:        "find closest call when no exact match",
			targetDelta: 0.20,
			isPut:       false,
			expected:    410.0, // 0.18 is closest to 0.20
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.findStrikeByDelta(options, tt.targetDelta, tt.isPut)
			if result != tt.expected {
				t.Errorf("findStrikeByDelta() = %.1f, want %.1f", result, tt.expected)
			}
		})
	}
}

func TestStrangleStrategy_findTargetExpiration(t *testing.T) {
	strategy := &StrangleStrategy{}

	// Test finding expiration 45 days out
	result := strategy.findTargetExpiration(45)

	// Parse the result
	expDate, err := time.Parse("2006-01-02", result)
	if err != nil {
		t.Fatalf("findTargetExpiration() returned invalid date format: %s", result)
	}

	// Should be a Friday
	if expDate.Weekday() != time.Friday {
		t.Errorf("findTargetExpiration() returned %s (%s), want Friday",
			result, expDate.Weekday())
	}

	// Should be approximately 45 days from now (allow some variance for weekends)
	now := time.Now()
	daysDiff := int(expDate.Sub(now).Hours() / 24)
	if daysDiff < 40 || daysDiff > 50 {
		t.Errorf("findTargetExpiration(45) is %d days away, want ~45 days", daysDiff)
	}
}

// Helper function for substring matching
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsInMiddle(s, substr))))
}

func containsInMiddle(s, substr string) bool {
	for i := 1; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Mock Broker for testing
type mockBroker struct {
	optionPrices map[string]map[float64]map[string]float64
	balance      float64
}

// Compile-time interface compliance check
var _ broker.Broker = (*mockBroker)(nil)

func newMockBroker(balance float64) *mockBroker {
	return &mockBroker{
		balance:      balance,
		optionPrices: make(map[string]map[float64]map[string]float64),
	}
}

func (m *mockBroker) setOptionPrice(expiration string, strike float64, optionType string, midPrice float64) {
	if m.optionPrices[expiration] == nil {
		m.optionPrices[expiration] = make(map[float64]map[string]float64)
	}
	if m.optionPrices[expiration][strike] == nil {
		m.optionPrices[expiration][strike] = make(map[string]float64)
	}
	m.optionPrices[expiration][strike][optionType] = midPrice
}

func (m *mockBroker) GetAccountBalance() (float64, error) {
	return m.balance, nil
}

func (m *mockBroker) GetPositions() ([]broker.PositionItem, error) {
	return nil, nil
}

func (m *mockBroker) GetQuote(symbol string) (*broker.QuoteItem, error) {
	return &broker.QuoteItem{Last: 420.0}, nil
}

func (m *mockBroker) GetExpirations(symbol string) ([]string, error) {
	return nil, nil
}

func (m *mockBroker) GetOptionChain(symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	var options []broker.Option

	// Check if we have custom prices set for this expiration
	if exp, exists := m.optionPrices[expiration]; exists {
		for strike, types := range exp {
			for optionType, midPrice := range types {
				// Create bid/ask around the mid price (±0.05)
				bid := midPrice - 0.05
				ask := midPrice + 0.05
				options = append(options, broker.Option{
					Strike:     strike,
					OptionType: optionType,
					Bid:        bid,
					Ask:        ask,
				})
			}
		}
	}

	// If no custom prices, return default test data
	if len(options) == 0 {
		options = []broker.Option{
			{
				Strike:     400.0,
				OptionType: "put",
				Bid:        0.80,
				Ask:        0.90, // Mid = 0.85
			},
			{
				Strike:     420.0,
				OptionType: "call",
				Bid:        0.85,
				Ask:        0.95, // Mid = 0.90
			},
		}
	}

	return options, nil
}

func (m *mockBroker) PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string, quantity int, limitPrice float64, preview bool) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string, quantity int, credit, profitTarget float64) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string, quantity int, maxDebit float64) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) GetOrderStatus(orderID int) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{
		Order: struct {
			CreateDate        string  `json:"create_date"`
			Type              string  `json:"type"`
			Symbol            string  `json:"symbol"`
			Side              string  `json:"side"`
			Class             string  `json:"class"`
			Status            string  `json:"status"`
			Duration          string  `json:"duration"`
			TransactionDate   string  `json:"transaction_date"`
			AvgFillPrice      float64 `json:"avg_fill_price"`
			ExecQuantity      float64 `json:"exec_quantity"`
			LastFillPrice     float64 `json:"last_fill_price"`
			LastFillQuantity  float64 `json:"last_fill_quantity"`
			RemainingQuantity float64 `json:"remaining_quantity"`
			ID                int     `json:"id"`
			Price             float64 `json:"price"`
			Quantity          float64 `json:"quantity"`
		}{
			ID:     orderID,
			Status: "filled",
		},
	}, nil
}

func (m *mockBroker) PlaceBuyToCloseOrder(optionSymbol string, quantity int, maxPrice float64) (*broker.OrderResponse, error) {
	return nil, nil
}
