// Package util provides common utility functions for price calculations.
package util

import "math"

// RoundToTick rounds x to the nearest tick increment.
// Ties are rounded away from zero (per math.Round). For example, with tick=0.01:
// 1.2345 -> 1.23, 1.2350 -> 1.24.
func RoundToTick(x, tick float64) float64 {
	// Guard non-finite inputs
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	if tick == 0 {
		return x // Zero tick returns input unchanged
	}
	if math.IsNaN(tick) || math.IsInf(tick, 0) {
		return math.NaN()
	}
	t := math.Abs(tick)
	return math.Round(x/t) * t
}

// FloorToTick rounds down to the nearest tick (use for sell credits).
func FloorToTick(x, tick float64) float64 {
	// Guard non-finite inputs
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	if tick == 0 {
		return x // Zero tick returns input unchanged
	}
	if math.IsNaN(tick) || math.IsInf(tick, 0) {
		return math.NaN()
	}
	t := math.Abs(tick)
	r := x / t
	// Avoid dropping a tick when r is just below an integer due to FP representation.
	r = math.Nextafter(r, math.Inf(1))
	return math.Floor(r) * t
}

// CeilToTick rounds up to the nearest tick (use for buy debits).
func CeilToTick(x, tick float64) float64 {
	// Guard non-finite inputs
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	if tick == 0 {
		return x // Zero tick returns input unchanged
	}
	if math.IsNaN(tick) || math.IsInf(tick, 0) {
		return math.NaN()
	}
	t := math.Abs(tick)
	r := x / t
	r = math.Nextafter(r, math.Inf(-1))
	return math.Ceil(r) * t
}
