//go:build test

// +build test

package main

import (
	"fmt"
)

// extractUnderlyingFromOSI extracts the underlying symbol from an OSI-formatted option symbol
// e.g., "SPY241220P00450000" -> "SPY"
func extractUnderlyingFromOSI(s string) string {
	// OSI format: UNDERLYING + YYMMDD + P/C + 8-digit strike
	// We need to find the start of the 6-digit expiration date
	if len(s) < 15 { // minimum length for a valid option symbol
		return ""
	}

	// Look for the first 6-digit sequence (expiration date)
	for i := 0; i <= len(s)-6; i++ {
		if isSixDigits(s[i : i+6]) {
			return s[:i]
		}
	}

	return ""
}

// isSixDigits checks if a string consists of exactly 6 digits
func isSixDigits(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func main() {
	testCases := []string{
		"SPY241220P00450000",
		"AAPL241220C00150000",
		"TSLA241220P00800000",
		"QQQ241220C00400000",
		"INVALID",
		"TOOLONG241220P00450000EXTRA",
	}

	for _, test := range testCases {
		result := extractUnderlyingFromOSI(test)
		fmt.Printf("Input: %s -> Output: %s\n", test, result)
	}
}
