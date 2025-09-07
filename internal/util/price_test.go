package util

import (
	"math"
	"testing"
)

const tol = 1e-10

func almostEq(a, b float64) bool { return math.Abs(a-b) <= tol }

func TestRoundToTick(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		x        float64
		tick     float64
		expected float64
	}{
		{
			name:     "basic rounding down",
			x:        1.2345,
			tick:     0.01,
			expected: 1.23,
		},
		{
			name:     "tie rounds away from zero",
			x:        1.235,
			tick:     0.01,
			expected: 1.24,
		},
		{
			name:     "negative tie rounds away from zero",
			x:        -1.235,
			tick:     0.01,
			expected: -1.24,
		},
		{
			name:     "negative basic rounding",
			x:        -1.2345,
			tick:     0.01,
			expected: -1.23,
		},
		{
			name:     "larger tick size",
			x:        1.27,
			tick:     0.05,
			expected: 1.25,
		},
		{
			name:     "exact multiple",
			x:        1.25,
			tick:     0.05,
			expected: 1.25,
		},
		{
			name:     "tick larger than magnitude",
			x:        0.004,
			tick:     0.01,
			expected: 0.00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RoundToTick(tt.x, tt.tick)
			if !almostEq(result, tt.expected) {
				t.Errorf("RoundToTick(%v, %v) = %v, expected %v", tt.x, tt.tick, result, tt.expected)
			}
		})
	}
}

func TestFloorToTick(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		x        float64
		tick     float64
		expected float64
	}{
		{
			name:     "exact multiple",
			x:        1.30,
			tick:     0.05,
			expected: 1.30,
		},
		{
			name:     "float precision boundary - just below",
			x:        1.2999999999999,
			tick:     0.05,
			expected: 1.25,
		},
		{
			name:     "just above tick boundary",
			x:        1.2500000000001,
			tick:     0.05,
			expected: 1.25,
		},
		{
			name:     "basic floor",
			x:        1.237,
			tick:     0.01,
			expected: 1.23,
		},
		{
			name:     "negative values",
			x:        -1.237,
			tick:     0.01,
			expected: -1.24,
		},
		{
			name:     "negative exact multiple",
			x:        -1.25,
			tick:     0.05,
			expected: -1.25,
		},
		{
			name:     "negative boundary - just above",
			x:        -1.2500000000001,
			tick:     0.05,
			expected: -1.30,
		},
		{
			name:     "negative tick uses absolute value",
			x:        1.237,
			tick:     -0.01,
			expected: 1.23,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FloorToTick(tt.x, tt.tick)
			if !almostEq(result, tt.expected) {
				t.Errorf("FloorToTick(%v, %v) = %v, expected %v", tt.x, tt.tick, result, tt.expected)
			}
		})
	}
}

func TestCeilToTick(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		x        float64
		tick     float64
		expected float64
	}{
		{
			name:     "exact multiple",
			x:        1.30,
			tick:     0.05,
			expected: 1.30,
		},
		{
			name:     "float precision boundary - just above",
			x:        1.2500000000001,
			tick:     0.05,
			expected: 1.30,
		},
		{
			name:     "just below tick boundary",
			x:        1.2999999999999,
			tick:     0.05,
			expected: 1.30,
		},
		{
			name:     "basic ceil",
			x:        1.231,
			tick:     0.01,
			expected: 1.24,
		},
		{
			name:     "negative values",
			x:        -1.231,
			tick:     0.01,
			expected: -1.23,
		},
		{
			name:     "negative exact multiple",
			x:        -1.25,
			tick:     0.05,
			expected: -1.25,
		},
		{
			name:     "negative boundary - just below",
			x:        -1.2999999999999,
			tick:     0.05,
			expected: -1.25,
		},
		{
			name:     "negative tick uses absolute value",
			x:        -1.231,
			tick:     -0.01,
			expected: -1.23,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CeilToTick(tt.x, tt.tick)
			if !almostEq(result, tt.expected) {
				t.Errorf("CeilToTick(%v, %v) = %v, expected %v", tt.x, tt.tick, result, tt.expected)
			}
		})
	}
}

func TestTickRoundingEdgeCases(t *testing.T) {
	t.Run("zero tick returns input", func(t *testing.T) {
		input := 1.2345
		if result := RoundToTick(input, 0); result != input {
			t.Errorf("RoundToTick(%v, 0) = %v, expected %v", input, result, input)
		}
		if result := FloorToTick(input, 0); result != input {
			t.Errorf("FloorToTick(%v, 0) = %v, expected %v", input, result, input)
		}
		if result := CeilToTick(input, 0); result != input {
			t.Errorf("CeilToTick(%v, 0) = %v, expected %v", input, result, input)
		}
	})

	t.Run("NaN inputs return unchanged", func(t *testing.T) {
		nan := math.NaN()
		if result := RoundToTick(nan, 0.01); !math.IsNaN(result) {
			t.Errorf("RoundToTick(NaN, 0.01) = %v, expected NaN", result)
		}
		if result := FloorToTick(nan, 0.01); !math.IsNaN(result) {
			t.Errorf("FloorToTick(NaN, 0.01) = %v, expected NaN", result)
		}
		if result := CeilToTick(nan, 0.01); !math.IsNaN(result) {
			t.Errorf("CeilToTick(NaN, 0.01) = %v, expected NaN", result)
		}
	})

	t.Run("infinite inputs return unchanged", func(t *testing.T) {
		posInf := math.Inf(1)
		negInf := math.Inf(-1)

		if result := RoundToTick(posInf, 0.01); result != posInf {
			t.Errorf("RoundToTick(+Inf, 0.01) = %v, expected +Inf", result)
		}
		if result := RoundToTick(negInf, 0.01); result != negInf {
			t.Errorf("RoundToTick(-Inf, 0.01) = %v, expected -Inf", result)
		}
		if result := FloorToTick(posInf, 0.01); result != posInf {
			t.Errorf("FloorToTick(+Inf, 0.01) = %v, expected +Inf", result)
		}
		if result := FloorToTick(negInf, 0.01); result != negInf {
			t.Errorf("FloorToTick(-Inf, 0.01) = %v, expected -Inf", result)
		}
		if result := CeilToTick(posInf, 0.01); result != posInf {
			t.Errorf("CeilToTick(+Inf, 0.01) = %v, expected +Inf", result)
		}
		if result := CeilToTick(negInf, 0.01); result != negInf {
			t.Errorf("CeilToTick(-Inf, 0.01) = %v, expected -Inf", result)
		}
	})

	t.Run("negative tick uses absolute value", func(t *testing.T) {
		result := RoundToTick(1.235, -0.01)
		expected := 1.24
		if !almostEq(result, expected) {
			t.Errorf("RoundToTick(1.235, -0.01) = %v, expected %v", result, expected)
		}
	})

	t.Run("NaN tick yields NaN", func(t *testing.T) {
		nan := math.NaN()
		if result := RoundToTick(1.23, nan); !math.IsNaN(result) {
			t.Errorf("RoundToTick(1.23, NaN) = %v, expected NaN", result)
		}
		if result := FloorToTick(1.23, nan); !math.IsNaN(result) {
			t.Errorf("FloorToTick(1.23, NaN) = %v, expected NaN", result)
		}
		if result := CeilToTick(1.23, nan); !math.IsNaN(result) {
			t.Errorf("CeilToTick(1.23, NaN) = %v, expected NaN", result)
		}
	})
}
