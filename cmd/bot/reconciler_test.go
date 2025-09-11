package main

import (
	"testing"
)

func TestExtractUnderlyingFromSymbol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		symbol   string
		expected string
	}{
		// Standard option symbols with various expiration years
		{"SPY option 2024", "SPY240315C00610000", "SPY"},
		{"SPY option 2025", "SPY250315C00610000", "SPY"},
		{"SPY option 2026", "SPY260315C00610000", "SPY"},
		{"SPY option 2027", "SPY270315C00610000", "SPY"},
		{"SPY option 2028", "SPY280315C00610000", "SPY"},
		{"SPY option 2029", "SPY290315C00610000", "SPY"},
		{"SPY option 2030", "SPY300315C00610000", "SPY"},
		
		// Various ticker lengths
		{"3-char ticker", "QQQ250101P00450000", "QQQ"},
		{"4-char ticker", "AAPL250101C00150000", "AAPL"},
		{"5-char ticker", "GOOGL250101C01500000", "GOOGL"},
		
		// Stock symbols (no 6-digit date pattern)
		{"Stock SPY", "SPY", "SPY"},
		{"Stock AAPL", "AAPL", "AAPL"},
		{"Stock GOOGL", "GOOGL", "GOOGL"},
		{"Stock with numbers", "BRK.B", "BRK.B"},
		
		// Edge cases
		{"Empty string", "", ""},
		{"Short string", "AB", "AB"},
		{"5 digits only", "12345", "12345"},
		{"6 digits at start", "123456", "123456"}, // No ticker before date
		{"Ticker then 6 digits", "ABC123456", "ABC"},
		{"Ticker with special chars", "BRK.B250101C00450000", "BRK.B"},
		
		// Options with different date formats (still 6 digits)
		{"January 2025", "SPY250101C00500000", "SPY"},
		{"December 2024", "SPY241231P00400000", "SPY"},
		{"February 29 leap year", "SPY240229C00450000", "SPY"},
		
		// Symbols that might have looked like years but aren't in date position
		{"Symbol with 240 in name", "A240BC", "A240BC"}, // Not 6 consecutive digits
		{"Symbol with 250 in middle", "AB250CD", "AB250CD"}, // Not 6 consecutive digits  
		{"Symbol with 260 at end", "ABC260", "ABC260"}, // Only 3 digits after
		
		// Complex cases
		{"Long ticker before date", "LONGNAME250315C00100000", "LONGNAME"},
		{"Numbers in ticker but valid date", "3M250315C00100000", "3M"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := extractUnderlyingFromSymbol(tt.symbol)
			if result != tt.expected {
				t.Errorf("extractUnderlyingFromSymbol(%q) = %q, want %q", tt.symbol, result, tt.expected)
			}
		})
	}
}

func TestExtractUnderlyingFromSymbol_BoundaryChecks(t *testing.T) {
	t.Parallel()
	// Test that the function doesn't panic with various edge cases
	edgeCases := []string{
		"",           // Empty
		"A",          // Single char
		"AB",         // Two chars
		"ABC",        // Three chars
		"ABCD",       // Four chars
		"ABCDE",      // Five chars
		"123456",     // Exactly 6 digits at start
		"A123456",    // One char then 6 digits
		"AB123456",   // Two chars then 6 digits
		"ABC123456",  // Three chars then 6 digits
		"123456ABC",  // 6 digits then chars
		"12345",      // Only 5 digits
		"1234567",    // 7 digits
		" SPY250101C00500000", // Leading space
		"SPY250101C00500000 ", // Trailing space
		"BRK.B 250101C00450000", // Space before date (should not panic)
	}
	
	for _, symbol := range edgeCases {
		t.Run(symbol, func(t *testing.T) {
			t.Parallel()
			// This should not panic
			_ = extractUnderlyingFromSymbol(symbol)
		})
	}
}