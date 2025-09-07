package broker

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/sony/gobreaker"
)

func TestCalculateIVR(t *testing.T) {
	tests := []struct {
		name         string
		historicalIV []float64
		currentIV    float64
		expected     float64
	}{
		{
			name:         "normal range",
			currentIV:    25.0,
			historicalIV: []float64{10.0, 15.0, 20.0, 25.0, 30.0, 35.0, 40.0},
			expected:     50.0, // (25-10)/(40-10) * 100 = 50
		},
		{
			name:         "at minimum",
			currentIV:    10.0,
			historicalIV: []float64{10.0, 20.0, 30.0},
			expected:     0.0, // (10-10)/(30-10) * 100 = 0
		},
		{
			name:         "at maximum",
			currentIV:    30.0,
			historicalIV: []float64{10.0, 20.0, 30.0},
			expected:     100.0, // (30-10)/(30-10) * 100 = 100
		},
		{
			name:         "no range (all same)",
			currentIV:    20.0,
			historicalIV: []float64{20.0, 20.0, 20.0},
			expected:     0.0, // Return 0 when min=max (no volatility range)
		},
		{
			name:         "empty history",
			currentIV:    20.0,
			historicalIV: []float64{},
			expected:     0.0,
		},
		{
			name:         "high IV rank",
			currentIV:    35.0,
			historicalIV: []float64{15.0, 20.0, 25.0, 30.0, 40.0},
			expected:     80.0, // (35-15)/(40-15) * 100 = 80
		},
		{
			name:         "monotonic bounds - current IV below historical min",
			currentIV:    5.0,
			historicalIV: []float64{10.0, 15.0, 20.0, 25.0, 30.0},
			expected:     0.0, // Should clamp to 0 when current IV < min historical
		},
		{
			name:         "monotonic bounds - current IV above historical max",
			currentIV:    50.0,
			historicalIV: []float64{10.0, 15.0, 20.0, 25.0, 30.0},
			expected:     100.0, // Should clamp to 100 when current IV > max historical
		},
		{
			name:         "monotonic bounds - negative current IV",
			currentIV:    -5.0,
			historicalIV: []float64{10.0, 15.0, 20.0, 25.0, 30.0},
			expected:     0.0, // Should clamp to 0 for negative current IV
		},
		{
			name:         "monotonic bounds - extreme high current IV",
			currentIV:    1000.0,
			historicalIV: []float64{10.0, 15.0, 20.0, 25.0, 30.0},
			expected:     100.0, // Should clamp to 100 for extremely high current IV
		},
		{
			name:         "robustness - current IV is NaN",
			currentIV:    math.NaN(),
			historicalIV: []float64{10.0, 15.0, 20.0, 25.0, 30.0},
			expected:     0.0, // Should return 0 for NaN current IV
		},
		{
			name:         "robustness - current IV is +Inf",
			currentIV:    math.Inf(1),
			historicalIV: []float64{10.0, 15.0, 20.0, 25.0, 30.0},
			expected:     100.0, // Should clamp to 100 for +Inf current IV
		},
		{
			name:         "robustness - current IV is -Inf",
			currentIV:    math.Inf(-1),
			historicalIV: []float64{10.0, 15.0, 20.0, 25.0, 30.0},
			expected:     0.0, // Should clamp to 0 for -Inf current IV
		},
		{
			name:         "robustness - historical IV contains NaN",
			currentIV:    20.0,
			historicalIV: []float64{10.0, math.NaN(), 20.0, 25.0, 30.0},
			expected:     50.0, // Should filter out NaN and compute: (20-10)/(30-10) * 100 = 50
		},
		{
			name:         "robustness - historical IV contains Inf",
			currentIV:    20.0,
			historicalIV: []float64{10.0, math.Inf(1), 20.0, 25.0, 30.0},
			expected:     50.0, // Should filter out Inf and compute: (20-10)/(30-10) * 100 = 50
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateIVR(tt.currentIV, tt.historicalIV)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("CalculateIVR(%v, %v) = %v, want %v",
					tt.currentIV, tt.historicalIV, result, tt.expected)
			}
		})
	}
}

func TestGetOptionByStrike(t *testing.T) {
	options := []Option{
		{Strike: 400.0, OptionType: "put", Symbol: "SPY240301P00400000"},
		{Strike: 420.0, OptionType: "call", Symbol: "SPY240301C00420000"},
		{Strike: 410.0, OptionType: "put", Symbol: "SPY240301P00410000"},
		{Strike: 430.0, OptionType: "call", Symbol: "SPY240301C00430000"},
	}

	tests := []struct {
		expected   *Option
		name       string
		optionType OptionType
		strike     float64
	}{
		{
			name:       "find put option",
			strike:     400.0,
			optionType: OptionTypePut,
			expected:   &options[0],
		},
		{
			name:       "find call option",
			strike:     420.0,
			optionType: OptionTypeCall,
			expected:   &options[1],
		},
		{
			name:       "option not found",
			strike:     500.0,
			optionType: OptionTypePut,
			expected:   nil,
		},
		{
			name:       "wrong type",
			strike:     400.0,
			optionType: OptionTypeCall,
			expected:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetOptionByStrike(options, tt.strike, tt.optionType)
			if result != tt.expected {
				t.Errorf("GetOptionByStrike() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAbsDaysBetween(t *testing.T) {
	tests := []struct {
		from     time.Time
		to       time.Time
		name     string
		expected int
	}{
		{
			name:     "same day",
			from:     time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			to:       time.Date(2024, 1, 1, 15, 0, 0, 0, time.UTC),
			expected: 0,
		},
		{
			name:     "one day difference",
			from:     time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			to:       time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC),
			expected: 1,
		},
		{
			name:     "45 days (typical DTE)",
			from:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			to:       time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC),
			expected: 45,
		},
		{
			name:     "21 days (exit threshold)",
			from:     time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			to:       time.Date(2024, 2, 22, 0, 0, 0, 0, time.UTC),
			expected: 21,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AbsDaysBetween(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("AbsDaysBetween(%v, %v) = %v, want %v",
					tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

func TestTradierClient_PlaceStrangleOrder_ProfitTarget(t *testing.T) {
	// Verify that TradierClient is created with profitTarget parameter
	client, err := NewTradierClient("test", "test", true, true, 0.5)
	if err != nil {
		t.Fatalf("NewTradierClient failed: %v", err)
	}

	// Verify the client is not nil and has expected configuration
	if client == nil {
		t.Fatal("NewTradierClient returned nil")
	}

	// TODO: Add mock HTTP client to verify profitTarget is passed correctly in API calls
	// This would require refactoring TradierClient to accept an http.Client interface
}

func TestNewTradierClient_ProfitTargetValidation(t *testing.T) {
	tests := []struct {
		name              string
		inputProfitTarget float64
		expectError       bool
		expected          float64
	}{
		{
			name:              "valid profitTarget",
			inputProfitTarget: 0.5,
			expectError:       false,
			expected:          0.5,
		},
		{
			name:              "negative profitTarget - error",
			inputProfitTarget: -0.1,
			expectError:       true,
			expected:          0.0,
		},
		{
			name:              "profitTarget above 1.0 - error",
			inputProfitTarget: 1.5,
			expectError:       true,
			expected:          0.0,
		},
		{
			name:              "large negative profitTarget - error",
			inputProfitTarget: -10.0,
			expectError:       true,
			expected:          0.0,
		},
		{
			name:              "large positive profitTarget - error",
			inputProfitTarget: 100.0,
			expectError:       true,
			expected:          0.0,
		},
		{
			name:              "zero value - valid",
			inputProfitTarget: 0.0,
			expectError:       false,
			expected:          0.0,
		},
		{
			name:              "one value - valid",
			inputProfitTarget: 1.0,
			expectError:       false,
			expected:          1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewTradierClient("test", "test", true, true, tt.inputProfitTarget)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for profitTarget %v, but got none", tt.inputProfitTarget)
				}
				if client != nil {
					t.Errorf("expected nil client for invalid profitTarget, but got %v", client)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for valid profitTarget %v: %v", tt.inputProfitTarget, err)
				}
				if client == nil {
					t.Fatal("NewTradierClient returned nil for valid input")
				}
				if client.profitTarget != tt.expected {
					t.Errorf("profitTarget = %v, want %v", client.profitTarget, tt.expected)
				}
			}
		})
	}
}

func TestExtractUnderlyingFromOSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Valid OSI symbols with different underlying lengths
		{
			name:     "3-char underlying (SPY)",
			input:    "SPY241220P00450000",
			expected: "SPY",
		},
		{
			name:     "4-char underlying (TSLA)",
			input:    "TSLA241220P00250000",
			expected: "TSLA",
		},
		{
			name:     "4-char underlying (NVDA)",
			input:    "NVDA241220P00500000",
			expected: "NVDA",
		},
		{
			name:     "4-char underlying (AAPL)",
			input:    "AAPL241220C00150000",
			expected: "AAPL",
		},
		{
			name:     "7-char underlying (valid)",
			input:    "ABCDEFG241220P00450000",
			expected: "ABCDEFG",
		},
		// Edge cases and malformed strings
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "too short",
			input:    "SPY24",
			expected: "",
		},
		{
			name:     "missing expiration",
			input:    "SPYP00450000",
			expected: "",
		},
		{
			name:     "invalid expiration format",
			input:    "SPY24ABCP00450000",
			expected: "",
		},
		{
			name:     "missing type char",
			input:    "SPY24122000450000",
			expected: "",
		},
		{
			name:     "invalid type char",
			input:    "SPY241220X00450000",
			expected: "",
		},
		{
			name:     "missing strike",
			input:    "SPY241220P",
			expected: "",
		},
		{
			name:     "short strike",
			input:    "SPY241220P450000",
			expected: "",
		},
		{
			name:     "long strike",
			input:    "SPY241220P004500000",
			expected: "",
		},
		{
			name:     "strike with non-digits",
			input:    "SPY241220P00450ABC",
			expected: "",
		},
		{
			name:     "underlying with spaces",
			input:    "SP Y241220P00450000",
			expected: "SP Y",
		},
		{
			name:     "expiration with spaces",
			input:    "SPY24 220P00450000",
			expected: "",
		},
		{
			name:     "strike with spaces",
			input:    "SPY241220P004 0000",
			expected: "",
		},
		{
			name:     "leading spaces",
			input:    " SPY241220P00450000",
			expected: "SPY",
		},
		{
			name:     "trailing spaces",
			input:    "SPY241220P00450000 ",
			expected: "SPY",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "",
		},
		{
			name:     "mixed case underlying",
			input:    "SpY241220P00450000",
			expected: "SpY",
		},
		{
			name:     "numeric underlying with embedded date",
			input:    "123241220P00450000",
			expected: "",
		},
		{
			name:     "underlying with numbers",
			input:    "SPY1241220P00450000",
			expected: "",
		},
		{
			name:     "very long underlying",
			input:    "VERYLONGUNDERLYINGSYMBOL241220P00450000",
			expected: "VERYLONGUNDERLYINGSYMBOL",
		},
		{
			name:     "underlying with special chars",
			input:    "SPY$241220P00450000",
			expected: "SPY$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUnderlyingFromOSI(tt.input)
			if result != tt.expected {
				t.Errorf("extractUnderlyingFromOSI(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestOptionTypeFromSymbol(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Valid OSI symbols
		{
			name:     "put option",
			input:    "SPY241220P00450000",
			expected: "put",
		},
		{
			name:     "call option",
			input:    "SPY241220C00450000",
			expected: "call",
		},
		{
			name:     "put option with different strike",
			input:    "TSLA241220P00250000",
			expected: "put",
		},
		{
			name:     "call option with different strike",
			input:    "NVDA241220C00500000",
			expected: "call",
		},
		// Edge cases and malformed strings
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "too short",
			input:    "SPY",
			expected: "",
		},
		{
			name:     "missing 8 digits",
			input:    "SPY241220P",
			expected: "",
		},
		{
			name:     "7 digits",
			input:    "SPY241220P450000",
			expected: "",
		},
		{
			name:     "9 digits",
			input:    "SPY241220P004500000",
			expected: "",
		},
		{
			name:     "non-digit in strike",
			input:    "SPY241220P00450ABC",
			expected: "",
		},
		{
			name:     "missing type char",
			input:    "SPY24122000450000",
			expected: "",
		},
		{
			name:     "invalid type char",
			input:    "SPY241220X00450000",
			expected: "",
		},
		{
			name:     "spaces in strike",
			input:    "SPY241220P004 0000",
			expected: "",
		},
		{
			name:     "leading spaces",
			input:    " SPY241220P00450000",
			expected: "put",
		},
		{
			name:     "trailing spaces",
			input:    "SPY241220P00450000 ",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "        ",
			expected: "",
		},
		{
			name:     "mixed case type char",
			input:    "SPY241220p00450000",
			expected: "",
		},
		{
			name:     "very short string",
			input:    "P",
			expected: "",
		},
		{
			name:     "no underlying",
			input:    "241220P00450000",
			expected: "put",
		},
		{
			name:     "very long strike",
			input:    "SPY241220P123456789",
			expected: "",
		},
		{
			name:     "strike with leading zeros",
			input:    "SPY241220P00000000",
			expected: "put",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := optionTypeFromSymbol(tt.input)
			if result != tt.expected {
				t.Errorf("optionTypeFromSymbol(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsSixDigits(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid 6-digit strings
		{
			name:     "valid 6 digits - expiration date",
			input:    "241220",
			expected: true,
		},
		{
			name:     "valid 6 digits - all zeros",
			input:    "000000",
			expected: true,
		},
		{
			name:     "valid 6 digits - mixed",
			input:    "123456",
			expected: true,
		},
		// Invalid strings
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "too short - 5 digits",
			input:    "12345",
			expected: false,
		},
		{
			name:     "too long - 7 digits",
			input:    "1234567",
			expected: false,
		},
		{
			name:     "contains non-digits",
			input:    "12345A",
			expected: false,
		},
		{
			name:     "contains spaces",
			input:    "123 45",
			expected: false,
		},
		{
			name:     "contains special chars",
			input:    "123-45",
			expected: false,
		},
		{
			name:     "unicode digits",
			input:    "１２３４５６", // Full-width digits
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSixDigits(tt.input)
			if result != tt.expected {
				t.Errorf("isSixDigits(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsEightDigits(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid 8-digit strings
		{
			name:     "valid 8 digits - strike price",
			input:    "00450000",
			expected: true,
		},
		{
			name:     "valid 8 digits - all zeros",
			input:    "00000000",
			expected: true,
		},
		{
			name:     "valid 8 digits - mixed",
			input:    "12345678",
			expected: true,
		},
		{
			name:     "valid 8 digits - high value",
			input:    "99999999",
			expected: true,
		},
		// Invalid strings
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "too short - 7 digits",
			input:    "1234567",
			expected: false,
		},
		{
			name:     "too long - 9 digits",
			input:    "123456789",
			expected: false,
		},
		{
			name:     "contains non-digits",
			input:    "1234567A",
			expected: false,
		},
		{
			name:     "contains spaces",
			input:    "123456 7",
			expected: false,
		},
		{
			name:     "contains special chars",
			input:    "12345-67",
			expected: false,
		},
		{
			name:     "unicode digits",
			input:    "１２３４５６７８", // Full-width digits
			expected: false,
		},
		{
			name:     "mixed with letters",
			input:    "12345ABC",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEightDigits(tt.input)
			if result != tt.expected {
				t.Errorf("isEightDigits(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// MockBroker for testing CircuitBreakerBroker
type MockBroker struct {
	callCount  int
	shouldFail bool
	failAfter  int
}

func (m *MockBroker) GetAccountBalance() (float64, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return 0, errors.New("mock broker error")
	}
	return 1000.0, nil
}

func (m *MockBroker) GetOptionBuyingPower() (float64, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return 0, errors.New("mock broker error")
	}
	return 5000.0, nil
}

func (m *MockBroker) GetPositions() ([]PositionItem, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	return []PositionItem{}, nil
}

func (m *MockBroker) GetQuote(symbol string) (*QuoteItem, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	return &QuoteItem{Symbol: symbol, Last: 100.0}, nil
}

func (m *MockBroker) GetExpirations(_ string) ([]string, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	return []string{"2024-12-20"}, nil
}

func (m *MockBroker) GetOptionChain(_, _ string, _ bool) ([]Option, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	return []Option{}, nil
}

func (m *MockBroker) GetOptionChainCtx(_ context.Context, _, _ string, _ bool) ([]Option, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	return []Option{}, nil
}

func (m *MockBroker) PlaceStrangleOrder(_ string, _, _ float64, _ string,
	_ int, _ float64, _ bool, _ string, _ string) (*OrderResponse, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	resp := &OrderResponse{}
	resp.Order.ID = 123
	return resp, nil
}

func (m *MockBroker) PlaceStrangleOTOCO(_ string, _, _ float64, _ string,
	_ int, _, _ float64, _ bool) (*OrderResponse, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	resp := &OrderResponse{}
	resp.Order.ID = 123
	return resp, nil
}

func (m *MockBroker) GetOrderStatus(orderID int) (*OrderResponse, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	resp := &OrderResponse{}
	resp.Order.ID = orderID
	return resp, nil
}

func (m *MockBroker) GetOrderStatusCtx(_ context.Context, orderID int) (*OrderResponse, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	resp := &OrderResponse{}
	resp.Order.ID = orderID
	return resp, nil
}

func (m *MockBroker) CloseStranglePosition(_ string, _, _ float64, _ string,
	_ int, _ float64, _ string) (*OrderResponse, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	resp := &OrderResponse{}
	resp.Order.ID = 123
	return resp, nil
}

func (m *MockBroker) PlaceBuyToCloseOrder(_ string, _ int,
	_ float64, _ string) (*OrderResponse, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	resp := &OrderResponse{}
	resp.Order.ID = 123
	return resp, nil
}

func (m *MockBroker) GetMarketClock(_ bool) (*MarketClockResponse, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return nil, errors.New("mock broker error")
	}
	return &MarketClockResponse{
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

func (m *MockBroker) IsTradingDay(_ bool) (bool, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return false, errors.New("mock broker error")
	}
	return true, nil
}

func (m *MockBroker) GetTickSize(_ string) (float64, error) {
	m.callCount++
	if m.shouldFail && m.callCount > m.failAfter {
		return 0, errors.New("mock broker error")
	}
	return 0.01, nil
}

func TestNewCircuitBreakerBroker(t *testing.T) {
	mockBroker := &MockBroker{}
	cb := NewCircuitBreakerBroker(mockBroker)

	if cb == nil {
		t.Fatal("NewCircuitBreakerBroker returned nil")
	}
	if cb.broker != mockBroker {
		t.Error("CircuitBreakerBroker.broker not set correctly")
	}
	if cb.breaker == nil {
		t.Error("CircuitBreakerBroker.breaker not initialized")
	}
}

func TestCircuitBreakerBroker_SuccessfulCalls(t *testing.T) {
	mockBroker := &MockBroker{shouldFail: false}
	cb := NewCircuitBreakerBroker(mockBroker)

	// Test successful GetAccountBalance
	balance, err := cb.GetAccountBalance()
	if err != nil {
		t.Errorf("GetAccountBalance failed: %v", err)
	}
	if balance != 1000.0 {
		t.Errorf("GetAccountBalance returned %v, want 1000.0", balance)
	}

	// Test successful GetQuote
	quote, err := cb.GetQuote("SPY")
	if err != nil {
		t.Errorf("GetQuote failed: %v", err)
	}
	if quote.Symbol != "SPY" {
		t.Errorf("GetQuote returned symbol %s, want SPY", quote.Symbol)
	}
}

func TestCircuitBreakerBroker_FailureScenarios(t *testing.T) {
	mockBroker := &MockBroker{shouldFail: true, failAfter: 3}
	testSettings := CircuitBreakerSettings{
		MaxRequests:  1,
		Interval:     10 * time.Millisecond,
		Timeout:      20 * time.Millisecond,
		MinRequests:  1,
		FailureRatio: 0.5,
	}
	cb := NewCircuitBreakerBrokerWithSettings(mockBroker, testSettings)

	// Make several calls to trip the breaker
	for i := 0; i < 8; i++ {
		_, err := cb.GetAccountBalance()
		if i < 3 {
			// First 3 calls should succeed
			if err != nil {
				t.Errorf("Call %d should succeed but failed: %v", i+1, err)
			}
		} else {
			// Subsequent calls should fail
			if err == nil {
				t.Errorf("Call %d should fail but succeeded", i+1)
			}
		}
	}

	// Check that breaker is open
	if cb.breaker.State() != gobreaker.StateOpen {
		t.Errorf("Circuit breaker should be open, but state is %s", cb.breaker.State())
	}
}

func TestCircuitBreakerBroker_RecoveryBehavior(t *testing.T) {
	mockBroker := &MockBroker{shouldFail: true, failAfter: 3}
	// Use very fast settings for testing to avoid delays
	fastSettings := CircuitBreakerSettings{
		MaxRequests:  3,
		Interval:     10 * time.Millisecond,
		Timeout:      15 * time.Millisecond,
		MinRequests:  5,
		FailureRatio: 0.6,
	}
	cb := NewCircuitBreakerBrokerWithSettings(mockBroker, fastSettings)

	// Trip the breaker
	for i := 0; i < 8; i++ {
		_, _ = cb.GetAccountBalance() // Ignore errors during breaker tripping
	}

	// Verify breaker is open
	if cb.breaker.State() != gobreaker.StateOpen {
		t.Fatalf("Circuit breaker should be open, but state is %s", cb.breaker.State())
	}

	// Poll for state transition instead of fixed sleep - more reliable in CI
	timeout := time.After(50 * time.Millisecond) // Total timeout for polling
	ticker := time.NewTicker(1 * time.Millisecond) // Poll every 1ms

	for {
		select {
		case <-timeout:
			ticker.Stop()
			t.Fatalf("Circuit breaker did not transition to half-open within timeout")
		case <-ticker.C:
			if cb.breaker.State() == gobreaker.StateHalfOpen {
				ticker.Stop()
				goto halfOpen
			}
		}
	}

halfOpen:
	// Breaker should be half-open now, allow limited requests
	mockBroker.shouldFail = false // Make broker succeed again

	// First call in half-open state should succeed
	balance, err := cb.GetAccountBalance()
	if err != nil {
		t.Errorf("Recovery call should succeed but failed: %v", err)
	}
	if balance != 1000.0 {
		t.Errorf("Recovery call returned %v, want 1000.0", balance)
	}

	// Make a few more calls to ensure state transition completes
	for i := 0; i < 3; i++ {
		balance, err := cb.GetAccountBalance()
		if err != nil {
			t.Errorf("Call %d after recovery should succeed but failed: %v", i+1, err)
		}
		if balance != 1000.0 {
			t.Errorf("Call %d after recovery returned %v, want 1000.0", i+1, balance)
		}
	}

	// Poll for final state transition to closed
	timeout = time.After(50 * time.Millisecond)
	ticker = time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			ticker.Stop()
			t.Fatalf("Circuit breaker did not transition to closed within timeout")
		case <-ticker.C:
			if cb.breaker.State() == gobreaker.StateClosed {
				ticker.Stop()
				return // Success
			}
		}
	}
}

func TestCircuitBreakerBroker_AllMethods(t *testing.T) {
	mockBroker := &MockBroker{shouldFail: false}
	cb := NewCircuitBreakerBroker(mockBroker)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"GetAccountBalance", func() error { _, err := cb.GetAccountBalance(); return err }},
		{"GetPositions", func() error { _, err := cb.GetPositions(); return err }},
		{"GetQuote", func() error { _, err := cb.GetQuote("SPY"); return err }},
		{"GetExpirations", func() error { _, err := cb.GetExpirations("SPY"); return err }},
		{"GetOptionChain", func() error { _, err := cb.GetOptionChain("SPY", "2024-12-20", false); return err }},
		{"PlaceStrangleOrder", func() error {
			_, err := cb.PlaceStrangleOrder("SPY", 400, 420, "2024-12-20", 1, 2.0, false, "day", "")
			return err
		}},
		{"PlaceStrangleOTOCO", func() error {
			_, err := cb.PlaceStrangleOTOCO("SPY", 400, 420, "2024-12-20", 1, 2.0, 0.5, false)
			return err
		}},
		{"GetOrderStatus", func() error { _, err := cb.GetOrderStatus(123); return err }},
		{"GetOrderStatusCtx", func() error { _, err := cb.GetOrderStatusCtx(context.Background(), 123); return err }},
		{"CloseStranglePosition", func() error {
			_, err := cb.CloseStranglePosition("SPY", 400, 420, "2024-12-20", 1, 5.0, "")
			return err
		}},
		{"PlaceBuyToCloseOrder", func() error {
			_, err := cb.PlaceBuyToCloseOrder("SPY241220P00400000", 1, 5.0, "day")
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != nil {
				t.Errorf("%s failed: %v", tt.name, err)
			}
		})
	}
}

func TestCircuitBreakerBroker_CircuitBreakerError(t *testing.T) {
	mockBroker := &MockBroker{shouldFail: true, failAfter: 0}
	// Use deterministic settings for reliable test behavior
	testSettings := CircuitBreakerSettings{
		MaxRequests:  3,
		Interval:     10 * time.Millisecond,
		Timeout:      50 * time.Millisecond,
		MinRequests:  1,
		FailureRatio: 0.5,
	}
	cb := NewCircuitBreakerBrokerWithSettings(mockBroker, testSettings)

	// Trip the breaker immediately
	for i := 0; i < 8; i++ {
		_, _ = cb.GetAccountBalance() // Ignore errors during breaker tripping
	}

	// Next call should return circuit breaker error
	_, err := cb.GetAccountBalance()
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("Expected gobreaker.ErrOpenState but got: %v", err)
	}
}


func TestNormalizeDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		// Valid standard values
		{"valid day", "day", "day", false},
		{"valid gtc", "gtc", "gtc", false},

		// Case normalization
		{"uppercase day", "DAY", "day", false},
		{"mixed case gtc", "Gtc", "gtc", false},

		// Whitespace trimming
		{"leading spaces", " day", "day", false},
		{"trailing spaces", "day ", "day", false},
		{"both spaces", " gtc ", "gtc", false},

		// Common variants mapping
		{"good-til-cancelled", "good-til-cancelled", "gtc", false},
		{"goodtilcancelled", "goodtilcancelled", "gtc", false},
		{"GOOD-TIL-CANCELLED", "GOOD-TIL-CANCELLED", "gtc", false},

		// Invalid gtd variants (Tradier API doesn't support gtd)
		{"valid gtd", "gtd", "", true},
		{"mixed case gtd", "GtD", "", true},
		{"good-til-date", "good-til-date", "", true},
		{"goodtildate", "goodtildate", "", true},
		{"GOOD-TIL-DATE", "GOOD-TIL-DATE", "", true},

		// Invalid values
		{"empty string", "", "", true},
		{"invalid duration", "week", "", true},
		{"invalid duration with spaces", " week ", "", true},
		{"invalid duration mixed case", "Week", "", true},
		{"numeric duration", "30", "", true},
		{"special chars", "day!", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeDuration(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("normalizeDuration(%q) expected error but got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("normalizeDuration(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("normalizeDuration(%q) = %q, want %q", tt.input, result, tt.expected)
				}
			}
		})
	}
}
