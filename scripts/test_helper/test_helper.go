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
	if len(s) < 16 { // minimum length for a valid option symbol
		return ""
	}

	// Look for the first 6-digit sequence (expiration date) with proper validation
	for i := 0; i <= len(s)-15; i++ { // need at least 15 chars after start for YYMMDD + P/C + 8 digits
		if isSixDigits(s[i : i+6]) {
			// Check that the 6-digit sequence is not part of a longer numeric run
			if i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
				continue // previous char is digit, skip
			}

			// Check that 6-digit expiration is followed by P/C and exactly 8 digits
			if i+15 > len(s) {
				continue // not enough chars remaining
			}

			expirationEnd := i + 6
			typeChar := s[expirationEnd]
			if typeChar != 'P' && typeChar != 'C' {
				continue // not followed by P or C
			}

			strikeStart := expirationEnd + 1
			if !isEightDigits(s[strikeStart : strikeStart+8]) {
				continue // not followed by exactly 8 digits
			}

			// Check that the strike is not part of a longer numeric run
			strikeEnd := strikeStart + 8
			if strikeEnd < len(s) && s[strikeEnd] >= '0' && s[strikeEnd] <= '9' {
				continue // next char is digit, skip
			}

			// All conditions met, return underlying
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

// isEightDigits checks if a string consists of exactly 8 digits
func isEightDigits(s string) bool {
	if len(s) != 8 {
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
