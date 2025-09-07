// Package util provides common utility functions for price calculations.
package util

import "math"

// RoundToTick rounds x to the nearest tick increment.
// For example, with tick=0.01, 1.2345 becomes 1.23 or 1.24 depending on rounding.
func RoundToTick(x, tick float64) float64 {
	// Guard non-finite inputs and zero tick
	if tick == 0 || math.IsNaN(tick) || math.IsInf(tick, 0) || math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	t := math.Abs(tick)
	return math.Round(x/t) * t
}

// FloorToTick rounds down to the nearest tick (use for sell credits).
func FloorToTick(x, tick float64) float64 {
	if tick == 0 || math.IsNaN(tick) || math.IsInf(tick, 0) || math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	t := math.Abs(tick)
	return math.Floor(x/t) * t
}

// CeilToTick rounds up to the nearest tick (use for buy debits).
func CeilToTick(x, tick float64) float64 {
	if tick == 0 || math.IsNaN(tick) || math.IsInf(tick, 0) || math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	t := math.Abs(tick)
	return math.Ceil(x/t) * t
}
