// Unit tests for helper functions in scripts/test_tradier/test_tradier_test.go
// Testing library/framework: Go standard library "testing".
package main

import (
	"bytes"
	"io"
	"math"
	"os"
	"strings"
	"testing"
)

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	f()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func TestFormatNumber_BasicAndRounding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0K"},
		{1050, "1.1K"},
		{1499, "1.5K"},
		{1500, "1.5K"},
		{999999, "1000.0K"}, // edge rounding just below 1M
		{1000000, "1.0M"},
		{2550000, "2.6M"},
		{-1, "-1"}, // negatives fall through to default formatting
	}
	for _, tc := range tests {
		got := formatNumber(tc.n)
		if got != tc.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestAbsInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   int
		want int
	}{
		{-123456789, 123456789},
		{-1, 1},
		{0, 0},
		{1, 1},
		{42, 42},
	}
	for _, tc := range tests {
		got := absInt(tc.in)
		if got != tc.want {
			t.Errorf("absInt(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
	// Note: math.MinInt cannot be represented as positive with absInt due to two's-complement overflow.
}

func TestEq_WithEpsilon(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b, eps float64
		want      bool
		name      string
	}{
		{1.0, 1.0, 1e-9, true, "exact"},
		{1.0, 1.0 + 1e-12, 1e-9, true, "tiny delta within eps"},
		{1.0, 1.0 + 1e-3, 1e-9, false, "delta exceeds eps"},
		{1.0, 1.0 + 1e-6, 1e-6, true, "delta equals eps inclusive"},
		{math.NaN(), math.NaN(), 1.0, false, "NaN vs NaN"},
		{math.Inf(1), math.Inf(1), 1.0, false, "Inf vs Inf"},
		{math.Inf(1), 1.0, 1.0, false, "Inf vs finite"},
	}
	for _, tc := range tests {
		if got := eq(tc.a, tc.b, tc.eps); got != tc.want {
			t.Errorf("eq(%v,%v,%v) [%s] = %v, want %v", tc.a, tc.b, tc.eps, tc.name, got, tc.want)
		}
	}
}

func TestIsOptionSymbol_ValidCases(t *testing.T) {
	t.Parallel()
	valid := []string{
		"SPY240920P00450000",
		" spy240920c00500000 ",
		"A240101C00005000",
		"ABCDEF240101P01234567",
	}
	for _, s := range valid {
		if !isOptionSymbol(s) {
			t.Errorf("isOptionSymbol(%q) = false, want true", s)
		}
	}
}

func TestIsOptionSymbol_InvalidCases(t *testing.T) {
	t.Parallel()
	invalid := []string{
		"",                         // empty
		"   ",                      // whitespace only
		"AAPL",                     // equity symbol
		"SPY240920Z00450000",       // invalid CP letter
		"SPY24092P00450000",        // date too short (5)
		"SPY240920P0045000",        // strike too short (7)
		"SPY240920P004500001",      // strike too long (9)
		"ABCDEFG240920P00450000",   // ticker too long (7)
		"SPY240920P00A50000",       // non-digit in strike
		"SPY240920PC0045000",       // extra letter in the middle
	}
	for _, s := range invalid {
		if isOptionSymbol(s) {
			t.Errorf("isOptionSymbol(%q) = true, want false", s)
		}
	}
}

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"", "<redacted>"},
		{"abcd1234", "<redacted>"},
		{"123456789012", "1234...9012"},
		{"abcdefghijklmnop", "abcd...mnop"},
	}
	for _, tc := range tests {
		if got := maskAPIKey(tc.in); got != tc.want {
			t.Errorf("maskAPIKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPrettyPrint_SuccessAndError(t *testing.T) {

	type payload struct {
		A int    `json:"a"`
		B string `json:"b"`
	}

	out := captureOutput(func() {
		prettyPrint(payload{A: 1, B: "x"})
	})
	want := "{\n  \"a\": 1,\n  \"b\": \"x\"\n}\n"
	if out != want {
		t.Errorf("prettyPrint(payload) output mismatch.\nGot:\n%s\nWant:\n%s", out, want)
	}

	errOut := captureOutput(func() {
		prettyPrint(make(chan int)) // not JSON-marshalable
	})
	if !strings.Contains(errOut, "Error marshaling JSON:") {
		t.Errorf("prettyPrint(chan) expected error message; got: %q", errOut)
	}
}