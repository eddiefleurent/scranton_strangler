// Package util provides common utility functions for price calculations.
package util

import "math"

// RoundToTick rounds x to the nearest tick increment.
// For example, with tick=0.01, 1.2345 becomes 1.23 or 1.24 depending on rounding.
func RoundToTick(x, tick float64) float64 {
	if tick <= 0 {
		return x
	}
	return math.Round(x/tick) * tick
}
