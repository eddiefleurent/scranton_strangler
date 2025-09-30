package strategy

import (
	"context"
	"errors"
	"log"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
)

// mockBrokerForStrategy implements broker.Broker for strategy testing
type mockBrokerForStrategy struct {
	balance         float64
	quote           *broker.QuoteItem
	quoteError      error
	chain           []broker.Option
	chainError      error
	expirations     []string
	expirationsErr  error
	marketClock     *broker.MarketClockResponse
	marketClockErr  error
}

// Compile-time interface compliance check
var _ broker.Broker = (*mockBrokerForStrategy)(nil)

func (m *mockBrokerForStrategy) GetAccountBalance() (float64, error) {
	return m.balance, nil
}

func (m *mockBrokerForStrategy) GetAccountBalanceCtx(ctx context.Context) (float64, error) {
	return m.balance, nil
}

func (m *mockBrokerForStrategy) GetOptionBuyingPower() (float64, error) {
	return 5000.0, nil
}

func (m *mockBrokerForStrategy) GetOptionBuyingPowerCtx(ctx context.Context) (float64, error) {
	return 5000.0, nil
}

func (m *mockBrokerForStrategy) GetPositions() ([]broker.PositionItem, error) {
	return m.GetPositionsCtx(context.Background())
}

func (m *mockBrokerForStrategy) GetPositionsCtx(ctx context.Context) ([]broker.PositionItem, error) {
	return []broker.PositionItem{}, nil
}

func (m *mockBrokerForStrategy) GetQuote(symbol string) (*broker.QuoteItem, error) {
	return m.quote, m.quoteError
}

func (m *mockBrokerForStrategy) GetExpirations(symbol string) ([]string, error) {
	return m.expirations, m.expirationsErr
}

func (m *mockBrokerForStrategy) GetExpirationsCtx(ctx context.Context, symbol string) ([]string, error) {
	return m.GetExpirations(symbol)
}

func (m *mockBrokerForStrategy) GetOptionChain(symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	return m.chain, m.chainError
}

func (m *mockBrokerForStrategy) GetOptionChainCtx(ctx context.Context, symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	return m.chain, m.chainError
}

func (m *mockBrokerForStrategy) PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string, quantity int, limitPrice float64, preview bool, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string, quantity int, credit, profitTarget float64, preview bool, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) GetOrderStatus(orderID int) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) GetOrderStatusCtx(ctx context.Context, orderID int) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) CancelOrder(orderID int) (*broker.OrderResponse, error) {
	resp := &broker.OrderResponse{}
	resp.Order.ID = orderID
	resp.Order.Status = "canceled"
	return resp, nil
}

func (m *mockBrokerForStrategy) CancelOrderCtx(ctx context.Context, orderID int) (*broker.OrderResponse, error) {
	return m.CancelOrder(orderID)
}

func (m *mockBrokerForStrategy) GetOrders() (*broker.OrdersResponse, error) {
	return &broker.OrdersResponse{}, nil
}

func (m *mockBrokerForStrategy) GetOrdersCtx(ctx context.Context) (*broker.OrdersResponse, error) {
	return m.GetOrders()
}

func (m *mockBrokerForStrategy) CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string, quantity int, maxDebit float64, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) CloseStranglePositionCtx(ctx context.Context, symbol string, putStrike, callStrike float64, expiration string, quantity int, maxDebit float64, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) PlaceBuyToCloseOrder(optionSymbol string, quantity int, maxPrice float64, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) PlaceSellToCloseOrder(optionSymbol string, quantity int, maxPrice float64, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) PlaceBuyToCloseMarketOrder(optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) PlaceSellToCloseMarketOrder(optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForStrategy) PlaceBuyToCloseMarketOrderCtx(ctx context.Context, optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	return m.PlaceBuyToCloseMarketOrder(optionSymbol, quantity, duration, tag)
}

func (m *mockBrokerForStrategy) PlaceSellToCloseMarketOrderCtx(ctx context.Context, optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	return m.PlaceSellToCloseMarketOrder(optionSymbol, quantity, duration, tag)
}

func (m *mockBrokerForStrategy) GetMarketClock(delayed bool) (*broker.MarketClockResponse, error) {
	return m.marketClock, m.marketClockErr
}

func (m *mockBrokerForStrategy) IsTradingDay(delayed bool) (bool, error) {
	return true, nil
}

func (m *mockBrokerForStrategy) GetTickSize(symbol string) (float64, error) {
	return 0.01, nil
}

func (m *mockBrokerForStrategy) GetHistoricalData(symbol string, interval string, startDate, endDate time.Time) ([]broker.HistoricalDataPoint, error) {
	return []broker.HistoricalDataPoint{}, nil
}

func (m *mockBrokerForStrategy) GetMarketCalendar(month, year int) (*broker.MarketCalendarResponse, error) {
	return &broker.MarketCalendarResponse{}, nil
}

func (m *mockBrokerForStrategy) GetMarketCalendarCtx(ctx context.Context, month, year int) (*broker.MarketCalendarResponse, error) {
	return &broker.MarketCalendarResponse{}, nil
}

func TestNewStrangleStrategy(t *testing.T) {
	mockBroker := &mockBrokerForStrategy{}
	mockStorage := storage.NewMockStorage()
	logger := log.New(log.Writer(), "test: ", log.LstdFlags)

	config := &Config{
		Symbol:       "SPY",
		AllocationPct: 0.35,
		DTETarget:    45,
		MinIVPct:     30.0, // Set threshold to 30%
	}

	strategy := NewStrangleStrategy(mockBroker, config, logger, mockStorage)

	if strategy.broker != mockBroker {
		t.Error("broker not set correctly")
	}
	if strategy.config != config {
		t.Error("config not set correctly")
	}
	if strategy.logger == nil {
		t.Error("logger should not be nil")
	}
	if strategy.storage != mockStorage {
		t.Error("storage not set correctly")
	}
	if strategy.chainCache == nil {
		t.Error("chainCache should be initialized")
	}
}

func TestNewStrangleStrategy_NilLogger(t *testing.T) {
	mockBroker := &mockBrokerForStrategy{}
	mockStorage := storage.NewMockStorage()

	config := &Config{
		Symbol:       "SPY",
		AllocationPct: 0.35,
		DTETarget:    45,
		MinIVPct:     30.0, // Set threshold to 30%
	}

	strategy := NewStrangleStrategy(mockBroker, config, nil, mockStorage)

	if strategy.logger == nil {
		t.Error("logger should not be nil even when passed nil")
	}
}


func TestStrangleStrategy_CheckVolatilityThreshold(t *testing.T) {
	tests := []struct {
		name          string
		ivValue       float64
		expectCanTrade bool
	}{
		{"IV above threshold", 35.0, true},
		{"IV at threshold", 30.0, true},
		{"IV below threshold", 25.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockBroker := &mockBrokerForStrategy{
				quote:       &broker.QuoteItem{Last: 400.0},
				expirations: []string{"2024-12-20"},
				chain: []broker.Option{
					{
						Strike:     400.0,
						OptionType: "call",
						Bid:        2.0,
						Ask:        2.2,
						Greeks: &broker.Greeks{
							MidIV: tt.ivValue / 100.0, // Convert percentage to decimal
						},
					},
				},
			}
			mockStorage := storage.NewMockStorage()

			config := &Config{
				Symbol:       "SPY",
				AllocationPct: 0.35,
				DTETarget:    45,
				MinIVPct:     30.0, // Set threshold to 30%
			}

			strategy := NewStrangleStrategy(mockBroker, config, log.Default(), mockStorage)

			canTrade, _ := strategy.CheckVolatilityThreshold()

			if canTrade != tt.expectCanTrade {
				t.Errorf("CheckVolatilityThreshold() = %v, want %v", canTrade, tt.expectCanTrade)
			}
		})
	}
}

func TestStrangleStrategy_GetCurrentIV(t *testing.T) {
	// Set up mock with option chain containing Greeks data
	mockBroker := &mockBrokerForStrategy{
		quote: &broker.QuoteItem{Last: 400.0},
		expirations: []string{"2024-12-20"},
		chain: []broker.Option{
			{
				Strike:     400.0,
				OptionType: "call",
				Bid:        2.0,
				Ask:        2.2,
				Greeks: &broker.Greeks{
					MidIV: 0.25, // 25% IV
				},
			},
		},
	}
	mockStorage := storage.NewMockStorage()

	config := &Config{
		Symbol:       "SPY",
		AllocationPct: 0.35,
		DTETarget:    45,
		MinIVPct:     30.0,
	}

	strategy := NewStrangleStrategy(mockBroker, config, log.Default(), mockStorage)

	iv := strategy.GetCurrentIV()
	if iv <= 0 {
		t.Errorf("GetCurrentIV() = %v, want > 0", iv)
	}
	expectedIV := 25.0 // GetCurrentIV returns percentage
	if math.Abs(iv-expectedIV) > 0.001 {
		t.Errorf("GetCurrentIV() = %v, want %v", iv, expectedIV)
	}
}

func TestStrangleStrategy_GetCurrentIV_NoQuote(t *testing.T) {
	mockBroker := &mockBrokerForStrategy{
		quoteError: errors.New("quote error"),
	}
	mockStorage := storage.NewMockStorage()

	config := &Config{
		Symbol:       "SPY",
		AllocationPct: 0.35,
		DTETarget:    45,
		MinIVPct:     30.0, // Set threshold to 30%
	}

	strategy := NewStrangleStrategy(mockBroker, config, log.Default(), mockStorage)

	iv := strategy.GetCurrentIV()
	// When quote fails, GetCurrentIV should return 0
	if iv != 0 {
		t.Errorf("GetCurrentIV() = %v, want 0 when quote fails", iv)
	}
}

func TestStrangleStrategy_CalculatePnL(t *testing.T) {
	mockBroker := &mockBrokerForStrategy{
		chain: []broker.Option{
			{
				Strike:     395.0,
				OptionType: "put",
				Bid:        1.0,
				Ask:        1.2,
			},
			{
				Strike:     405.0,
				OptionType: "call",
				Bid:        1.0,
				Ask:        1.2,
			},
		},
		expirations: []string{"2024-12-20"},
	}
	mockStorage := storage.NewMockStorage()

	config := &Config{
		Symbol:       "SPY",
		AllocationPct: 0.35,
		DTETarget:    45,
		MinIVPct:     30.0, // Set threshold to 30%
	}

	strategy := NewStrangleStrategy(mockBroker, config, log.Default(), mockStorage)

	position := &models.Position{
		Symbol:        "SPY",
		PutStrike:     395.0,
		CallStrike:    405.0,
		CreditReceived: 2.50, // Stored as per-share credit
		Quantity:      1,
		Expiration:    time.Date(2024, 12, 20, 0, 0, 0, 0, time.UTC),
	}

	pnl := strategy.CalculatePnL(position)
	// With current prices at $2.20 total ($220) vs $250 credit, P&L should be positive
	// Credit: $250, Current value: $220, P&L: $30
	expectedPnL := 30.0
	if math.Abs(pnl-expectedPnL) > 0.01 { // Allow small floating point differences
		t.Errorf("CalculatePnL() = %v, want %v", pnl, expectedPnL)
	}
}

func TestStrangleStrategy_GetCurrentPositionValue(t *testing.T) {
	mockBroker := &mockBrokerForStrategy{
		chain: []broker.Option{
			{
				Strike:     395.0,
				OptionType: "put",
				Bid:        1.0,
				Ask:        1.2,
			},
			{
				Strike:     405.0,
				OptionType: "call",
				Bid:        1.0,
				Ask:        1.2,
			},
		},
	}
	mockStorage := storage.NewMockStorage()

	config := &Config{
		Symbol:       "SPY",
		AllocationPct: 0.35,
		DTETarget:    45,
		MinIVPct:     30.0, // Set threshold to 30%
	}

	strategy := NewStrangleStrategy(mockBroker, config, log.Default(), mockStorage)

	position := &models.Position{
		PutStrike:  395.0,
		CallStrike: 405.0,
		Quantity:   1,
	}

	value, err := strategy.GetCurrentPositionValue(position)
	if err != nil {
		t.Errorf("GetCurrentPositionValue() error = %v", err)
	}
	// Should return the mid price of both options
	expectedValue := 220.0 // (1.1 + 1.1) * 100 shares per contract
	if math.Abs(value-expectedValue) > 0.01 { // Allow small floating point differences
		t.Errorf("GetCurrentPositionValue() = %v, want %v", value, expectedValue)
	}
}

func TestStrangleStrategy_validateStrikeSelection(t *testing.T) {
	mockBroker := &mockBrokerForStrategy{}
	mockStorage := storage.NewMockStorage()

	config := &Config{
		Symbol:       "SPY",
		AllocationPct: 0.35,
		DTETarget:    45,
		MinIVPct:     30.0, // Set threshold to 30%
	}

	strategy := NewStrangleStrategy(mockBroker, config, log.Default(), mockStorage)

	tests := []struct {
		name        string
		putStrike   float64
		callStrike  float64
		underlying  float64
		expectError bool
	}{
		{"valid strangle", 395.0, 405.0, 400.0, false},
		{"strikes too close", 399.0, 401.0, 400.0, true}, // spread = 0.5% (too tight)
		{"spread too wide", 370.0, 430.0, 400.0, true},   // spread = 15% (too wide)
		{"inverted strikes", 405.0, 395.0, 400.0, true},  // put > call
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := strategy.validateStrikeSelection(tt.putStrike, tt.callStrike, tt.underlying)
			if (err != nil) != tt.expectError {
				t.Errorf("validateStrikeSelection() error = %v, expectError %v", err, tt.expectError)
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
			Ask:        1.60, // Spread = 0.10 >= 0.01 tick
		},
		{
			Strike:     400.0,
			OptionType: "call",
			Bid:        0.80,
			Ask:        0.90, // Spread = 0.10 >= 0.01 tick
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
		{
			Strike:     410.0,
			OptionType: "put",
			Bid:        1.5000,
			Ask:        1.5005, // 0.0005 < 0.01 tick - for tight spread test
		},
		{
			Strike:     430.0,
			OptionType: "call",
			Bid:        0.8000,
			Ask:        0.8005, // 0.0005 < 0.01 tick - for tight spread test
		},
	}

	strategy := &StrangleStrategy{
		logger:     log.Default(),
		chainCache: make(map[string]*optionChainCacheEntry),
	}

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
		{
			name:       "reject tight spreads (< 1 tick)",
			putStrike:  410.0,
			callStrike: 430.0,
			expected:   0.0,
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
	cfg := &Config{
		Symbol:          "SPY",
		ProfitTarget:    0.50, // 50%
		MaxDTE:          21,   // Default MaxDTE value
		MaxPositionLoss: 2.5,  // Allow up to 250% loss (same as default stop loss)
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
			expectedReason: "none",
		},
		{
			name: "profit target reached",
			position: &models.Position{
				Symbol:         "SPY",
				PutStrike:      400.0,
				CallStrike:     420.0,
				Expiration:     time.Now().AddDate(0, 0, 35),
				Quantity:       1,
				CreditReceived: 3.50, // Stored as per-share credit (total credit: $350)
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
				CreditReceived: 3.50, // Stored as per-share credit (total credit: $350)
				DTE:            21,   // At max DTE
			},
			expectedExit:   true,
			expectedReason: "time",
		},
		{
			name: "200% escalate loss triggered",
			position: &models.Position{
				Symbol:         "SPY",
				PutStrike:      400.0,
				CallStrike:     420.0,
				Expiration:     time.Now().AddDate(0, 0, 35),
				Quantity:       1,
				CreditReceived: 3.50, // Stored as per-share credit (total credit: $350)
				DTE:            35,     // Still have time but need escalation
			},
			expectedExit:   true,
			expectedReason: "escalate",
		},
		{
			name: "250% stop-loss triggered",
			position: &models.Position{
				Symbol:         "SPY",
				PutStrike:      400.0,
				CallStrike:     420.0,
				Expiration:     time.Now().AddDate(0, 0, 35),
				Quantity:       1,
				CreditReceived: 3.50, // Stored as per-share credit (total credit: $350)
				DTE:            35,     // Still have time but losses are too high
			},
			expectedExit:   true,
			expectedReason: "stop_loss",
		},
		{
			name: "no exit conditions met",
			position: &models.Position{
				Symbol:         "SPY",
				PutStrike:      400.0,
				CallStrike:     420.0,
				Expiration:     time.Now().AddDate(0, 0, 35),
				Quantity:       1,
				CreditReceived: 3.50, // Stored as per-share credit (total credit: $350)
				DTE:            35,   // Still have time
			},
			expectedExit:   false,
			expectedReason: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockBroker(100000.0)
			strategy := &StrangleStrategy{
				broker:     mockClient,
				config:     cfg,
				logger:     log.Default(),
				chainCache: make(map[string]*optionChainCacheEntry),
			}

			// Set up mock option prices based on test case
			if tt.position != nil {
				expiration := tt.position.Expiration.Format("2006-01-02")
				setupTestScenarioPrices(t, mockClient, expiration, tt.name)
			}

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

	strategy := &StrangleStrategy{
		config: &Config{
			MinVolume:       0, // Disable liquidity filtering in tests
			MinOpenInterest: 0,
		},
		logger:     log.Default(),
		chainCache: make(map[string]*optionChainCacheEntry),
	}

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
	// Create mock broker
	mockClient := newMockBroker(100000.0)

	cfg := &Config{
		Symbol: "SPY",
	}

	strategy := &StrangleStrategy{
		broker:     mockClient,
		config:     cfg,
		logger:     log.Default(),
		chainCache: make(map[string]*optionChainCacheEntry),
	}

	// Test finding expiration 45 days out
	result := strategy.findTargetExpiration(45)

	// Parse the result
	expDate, err := time.Parse("2006-01-02", result)
	if err != nil {
		t.Fatalf("findTargetExpiration() returned invalid date format: %s", result)
	}

	// Should be M/W/F (options expire on Monday, Wednesday, Friday)
	weekday := expDate.Weekday()
	if weekday != time.Monday && weekday != time.Wednesday && weekday != time.Friday {
		t.Errorf("findTargetExpiration() returned %s (%s), want Monday/Wednesday/Friday",
			result, expDate.Weekday())
	}

	// Should be approximately 45 days from now (allow some variance for weekends)
	now := time.Now()
	daysDiff := int(expDate.Sub(now).Hours() / 24)
	if daysDiff < 40 || daysDiff > 50 {
		t.Errorf("findTargetExpiration(45) is %d days away, want ~45 days", daysDiff)
	}
}

func TestStrangleStrategy_shouldFilterForLiquidity(t *testing.T) {
	tests := []struct {
		name            string
		minVolume       int64
		minOpenInterest int64
		option          broker.Option
		expected        bool
	}{
		{
			name:            "both thresholds disabled - no filtering",
			minVolume:       0,
			minOpenInterest: 0,
			option: broker.Option{
				Volume:       5,
				OpenInterest: 50,
			},
			expected: false,
		},
		{
			name:            "sufficient volume and OI - no filtering",
			minVolume:       100,
			minOpenInterest: 1000,
			option: broker.Option{
				Volume:       150,
				OpenInterest: 1500,
			},
			expected: false,
		},
		{
			name:            "insufficient volume - should filter",
			minVolume:       100,
			minOpenInterest: 1000,
			option: broker.Option{
				Volume:       50,  // Below threshold
				OpenInterest: 1500,
			},
			expected: true,
		},
		{
			name:            "insufficient open interest - should filter",
			minVolume:       100,
			minOpenInterest: 1000,
			option: broker.Option{
				Volume:       150,
				OpenInterest: 500, // Below threshold
			},
			expected: true,
		},
		{
			name:            "no data available (test scenario) - no filtering",
			minVolume:       100,
			minOpenInterest: 1000,
			option: broker.Option{
				Volume:       0,
				OpenInterest: 0,
			},
			expected: false,
		},
		{
			name:            "volume only configured - filters based on volume",
			minVolume:       100,
			minOpenInterest: 0,
			option: broker.Option{
				Volume:       50, // Below threshold
				OpenInterest: 500,
			},
			expected: true,
		},
		{
			name:            "OI only configured - filters based on OI",
			minVolume:       0,
			minOpenInterest: 1000,
			option: broker.Option{
				Volume:       150,
				OpenInterest: 500, // Below threshold
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := &StrangleStrategy{
				config: &Config{
					MinVolume:       tt.minVolume,
					MinOpenInterest: tt.minOpenInterest,
				},
				logger:     log.Default(),
				chainCache: make(map[string]*optionChainCacheEntry),
			}

			result := strategy.shouldFilterForLiquidity(tt.option)
			if result != tt.expected {
				t.Errorf("shouldFilterForLiquidity() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func containsSubstring(s, substr string) bool { return strings.Contains(s, substr) }

// setupTestScenarioPrices configures mock option prices for different test scenarios.
// This helper keeps tests concise and makes it easier to extend with new scenarios.
func setupTestScenarioPrices(t *testing.T, mockClient *mockBroker, expiration string, scenario string) {
	t.Helper()

	switch scenario {
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
	case "200% escalate loss triggered":
		// For -200% loss: credit 3.50, current value should be 10.50
		// P&L = $350 - $1050 = -$700 (-200% loss)
		mockClient.setOptionPrice(expiration, 400.0, "put", 5.00)
		mockClient.setOptionPrice(expiration, 420.0, "call", 5.50)
		// Total: 10.50, P&L = 3.50 - 10.50 = -7.00 per contract = -$700
	case "250% stop-loss triggered":
		// For -250% loss: credit 3.50, current value should be 12.25
		// P&L = $350 - $1225 = -$875 (-250% loss)
		mockClient.setOptionPrice(expiration, 400.0, "put", 6.00)
		mockClient.setOptionPrice(expiration, 420.0, "call", 6.25)
		// Total: 12.25, P&L = 3.50 - 12.25 = -8.75 per contract = -$875
	case "no exit conditions met":
		// Same as max DTE but different DTE in position
		mockClient.setOptionPrice(expiration, 400.0, "put", 1.50)
		mockClient.setOptionPrice(expiration, 420.0, "call", 1.50)
		// Total: 3.00, P&L = ($350 - $300) = $50 (14% profit)
	}
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

func (m *mockBroker) GetAccountBalanceCtx(ctx context.Context) (float64, error) {
	return m.balance, nil
}

func (m *mockBroker) GetPositions() ([]broker.PositionItem, error) {
	return m.GetPositionsCtx(context.Background())
}

func (m *mockBroker) GetPositionsCtx(ctx context.Context) ([]broker.PositionItem, error) {
	return nil, nil
}

func (m *mockBroker) GetQuote(_ string) (*broker.QuoteItem, error) {
	return &broker.QuoteItem{Last: 420.0}, nil
}

func (m *mockBroker) GetExpirations(_ string) ([]string, error) {
	var exps []string
	start := time.Now().UTC()
	end := start.AddDate(0, 0, 90)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		switch d.Weekday() {
		case time.Monday, time.Wednesday, time.Friday:
			exps = append(exps, d.Format("2006-01-02"))
		}
	}
	return exps, nil
}

func (m *mockBroker) GetExpirationsCtx(ctx context.Context, symbol string) ([]string, error) {
	return m.GetExpirations(symbol)
}

func (m *mockBroker) GetOptionChain(_, expiration string, _ bool) ([]broker.Option, error) {
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
				Underlying: "SPY",
				OptionType: "put",
				Bid:        0.80,
				Ask:        0.90, // Mid = 0.85
			},
			{
				Strike:     420.0,
				Underlying: "SPY",
				OptionType: "call",
				Bid:        0.85,
				Ask:        0.95, // Mid = 0.90
			},
		}
	}

	return options, nil
}

func (m *mockBroker) GetOptionChainCtx(_ context.Context, symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	return m.GetOptionChain(symbol, expiration, withGreeks)
}

func (m *mockBroker) PlaceStrangleOrder(
	_ string,
	_, _ float64,
	_ string,
	_ int,
	_ float64,
	_ bool,
	_ string,
	_ string,
) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) PlaceStrangleOTOCO(
	_ string,
	_, _ float64,
	_ string,
	_ int,
	_, _ float64,
	_ bool,
	_ string,
	_ string,
) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) CloseStranglePosition(
	_ string,
	_, _ float64,
	_ string,
	_ int,
	_ float64,
	_ string,
) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) CloseStranglePositionCtx(
	_ context.Context,
	_ string,
	_, _ float64,
	_ string,
	_ int,
	_ float64,
	_ string,
) (*broker.OrderResponse, error) {
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

func (m *mockBroker) GetOrderStatusCtx(_ context.Context, orderID int) (*broker.OrderResponse, error) {
	return m.GetOrderStatus(orderID)
}

func (m *mockBroker) CancelOrder(orderID int) (*broker.OrderResponse, error) {
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
			Status: "canceled",
		},
	}, nil
}

func (m *mockBroker) CancelOrderCtx(_ context.Context, orderID int) (*broker.OrderResponse, error) {
	return m.CancelOrder(orderID)
}

func (m *mockBroker) GetOrders() (*broker.OrdersResponse, error) {
	return &broker.OrdersResponse{}, nil
}

func (m *mockBroker) GetOrdersCtx(_ context.Context) (*broker.OrdersResponse, error) {
	return m.GetOrders()
}

func (m *mockBroker) PlaceBuyToCloseOrder(
	_ string,
	_ int,
	_ float64,
	_ string,
	_ string,
) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) PlaceSellToCloseOrder(
	_ string,
	_ int,
	_ float64,
	_ string,
	_ string,
) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) PlaceBuyToCloseMarketOrder(
	_ string,
	_ int,
	_ string,
	_ string,
) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) PlaceSellToCloseMarketOrder(
	_ string,
	_ int,
	_ string,
	_ string,
) (*broker.OrderResponse, error) {
	return nil, nil
}

func (m *mockBroker) PlaceBuyToCloseMarketOrderCtx(
	ctx context.Context,
	optionSymbol string,
	quantity int,
	duration string,
	tag string,
) (*broker.OrderResponse, error) {
	return m.PlaceBuyToCloseMarketOrder(optionSymbol, quantity, duration, tag)
}

func (m *mockBroker) PlaceSellToCloseMarketOrderCtx(
	ctx context.Context,
	optionSymbol string,
	quantity int,
	duration string,
	tag string,
) (*broker.OrderResponse, error) {
	return m.PlaceSellToCloseMarketOrder(optionSymbol, quantity, duration, tag)
}

func (m *mockBroker) GetMarketClock(_ bool) (*broker.MarketClockResponse, error) {
	return &broker.MarketClockResponse{
		Clock: struct {
			Date        string `json:"date"`
			Description string `json:"description"`
			State       string `json:"state"`
			Timestamp   int64  `json:"timestamp"`
			NextChange  string `json:"next_change"`
			NextState   string `json:"next_state"`
		}{
			Date:        "2024-01-01",
			Description: "Market is open",
			State:       "open",
			Timestamp:   1704067200,
			NextChange:  "16:00",
			NextState:   "postmarket",
		},
	}, nil
}

func (m *mockBroker) IsTradingDay(_ bool) (bool, error) {
	return true, nil
}

func (m *mockBroker) GetTickSize(_ string) (float64, error) {
	return 0.01, nil
}

func (m *mockBroker) GetOptionBuyingPower() (float64, error) {
	// Return a mock option buying power (typically less than account balance)
	return m.balance * 0.8, nil
}

func (m *mockBroker) GetOptionBuyingPowerCtx(ctx context.Context) (float64, error) {
	// Return a mock option buying power (typically less than account balance)
	return m.balance * 0.8, nil
}

func (m *mockBroker) GetHistoricalData(symbol string, interval string, startDate, endDate time.Time) ([]broker.HistoricalDataPoint, error) {
	return []broker.HistoricalDataPoint{
		{Date: time.Now().AddDate(0, 0, -1), Close: 35.0},
		{Date: time.Now().AddDate(0, 0, -2), Close: 36.0},
	}, nil
}

func (m *mockBroker) GetMarketCalendar(month, year int) (*broker.MarketCalendarResponse, error) {
	return &broker.MarketCalendarResponse{}, nil
}

func (m *mockBroker) GetMarketCalendarCtx(ctx context.Context, month, year int) (*broker.MarketCalendarResponse, error) {
	return &broker.MarketCalendarResponse{}, nil
}
