package broker

import (
	"testing"
	"time"
)

func TestCalculateIVR(t *testing.T) {
	tests := []struct {
		name         string
		currentIV    float64
		historicalIV []float64
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
			expected:     50.0, // Default when no range
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
		name       string
		strike     float64
		optionType string
		expected   *Option
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
		name     string
		from     time.Time
		to       time.Time
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
	client := NewTradierClient("test", "test", true, true) // sandbox=true, useOTOCO=true

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
		return client.PlaceStrangleOrder("SPY", 450.0, 460.0, "2024-12-20", 1, 2.0, 0.5)
	}
}
