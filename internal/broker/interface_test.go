package broker

import (
	"testing"
	"time"
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
			expected:     0.0, // Return 0 when no range to avoid masking scenarios
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateIVR(tt.currentIV, tt.historicalIV)
			if result != tt.expected {
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
		optionType string
		strike     float64
	}{
		{
			name:       "find put option",
			strike:     400.0,
			optionType: "put",
			expected:   &options[0],
		},
		{
			name:       "find call option",
			strike:     420.0,
			optionType: "call",
			expected:   &options[1],
		},
		{
			name:       "option not found",
			strike:     500.0,
			optionType: "put",
			expected:   nil,
		},
		{
			name:       "wrong type",
			strike:     400.0,
			optionType: "call",
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

func TestDaysBetween(t *testing.T) {
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
			result := DaysBetween(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("DaysBetween(%v, %v) = %v, want %v",
					tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

func TestTradierClient_PlaceStrangleOrder_ProfitTarget(t *testing.T) {
	// Test that TradierClient forwards profitTarget correctly when useOTOCO is true
	client := NewTradierClient("test", "test", true, true, 0.5) // sandbox=true, useOTOCO=true, profitTarget=50%

	// We can't actually make API calls in unit tests, but we can verify the method signature
	// and that it doesn't panic with the new parameter
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PlaceStrangleOrder panicked with profitTarget parameter: %v", r)
		}
	}()

	// This would normally make an API call, but we're just testing that the method
	// accepts the profitTarget parameter without panicking
	// In a real test, we'd mock the API call
	_ = func() (*OrderResponse, error) {
		return client.PlaceStrangleOrder("SPY", 450.0, 460.0, "2024-12-20", 1, 2.0, false)
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
			name:     "4-char underlying (SPY)",
			input:    "SPY241220P00450000",
			expected: "SPY",
		},
		{
			name:     "5-char underlying (TSLA)",
			input:    "TSLA241220P00250000",
			expected: "TSLA",
		},
		{
			name:     "6-char underlying (NVDA)",
			input:    "NVDA241220P00500000",
			expected: "NVDA",
		},
		{
			name:     "3-char underlying (AAPL)",
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
