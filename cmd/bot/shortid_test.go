package main

import (
	"testing"
	"testing/quick"
)

// TestShortID_TableDriven covers happy paths, boundaries, and edge cases.
func TestShortID_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"len_gt_8_ascii", "1234567890abcdef", "12345678"},
		{"len_eq_8_ascii", "12345678", "12345678"},
		{"len_lt_8_ascii", "abcd", "abcd"},
		{"empty_string", "", ""},
		{"len_gt_8_with_dash", "abc-def-ghi", "abc-def-"},
		// Multibyte characters:
		// 'é' is 2 bytes; 5 of them = 10 bytes; first 8 bytes correspond to 4 'é'
		{"unicode_multibyte_2bytes", "ééééé", "éééé"},
		// '🦊' is 4 bytes; 3 of them = 12 bytes; first 8 bytes correspond to 2 '🦊'
		{"unicode_multibyte_4bytes", "🦊🦊🦊", "🦊🦊"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shortID(tc.in)
			if got \!= tc.want {
				t.Fatalf("shortID(%q) = %q; want %q", tc.in, got, tc.want)
			}
			if len(got) > 8 {
				t.Fatalf("shortID(%q) length = %d; want <= 8", tc.in, len(got))
			}
		})
	}
}

// Property-based checks to validate invariants over a wide range of inputs.
func TestShortID_Properties_Quick(t *testing.T) {
	prop := func(s string) bool {
		got := shortID(s)
		// Invariant 1: Output length is never more than 8 bytes.
		if len(got) > 8 {
			return false
		}
		// Invariant 2: If input <= 8 bytes, output equals input (idempotent on short inputs).
		if len(s) <= 8 && got \!= s {
			return false
		}
		// Invariant 3: If input > 8 bytes, output equals first 8 bytes of input.
		if len(s) > 8 && got \!= s[:8] {
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 512}
	if err := quick.Check(prop, cfg); err \!= nil {
		t.Fatalf("property check failed: %v", err)
	}
}