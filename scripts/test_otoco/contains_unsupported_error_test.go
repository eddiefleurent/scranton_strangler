// Tests for containsUnsupportedError in scripts/test_otoco.
// Testing library/framework: Go standard library "testing" (no external assertions).
package main

import "testing"

func TestContainsUnsupportedError_PositiveCases(t *testing.T) {
  t.Parallel()
  cases := []string{
    "unsupported order type: OTOCO",
    "This feature is NOT SUPPORTED by the broker",
    "Order type not available in current account tier",
    "Broker error: invalid order type for OTO",
    "server responded: OCO not supported",
    "Execution failed: ONE-TRIGGERS-OTHER is not available on this venue",
    "Error placing OTO order: invalid order type",
    "Failed to place order; OTOCO is UNSUPPORTED",
    // Mixed noise around keywords
    "HTTP 400: bad request - invalid order type (OCO)",
    "broker: feature flag disabled; not supported for OTO at this time",
    // JSON-like payloads where message includes an indicator
    `{"status":400,"error":"Invalid Order Type: OTO not supported"}`,
  }

  for i, msg := range cases {
    if !containsUnsupportedError(msg) {
      t.Fatalf("case %d expected true for message: %q", i, msg)
    }
  }
}

func TestContainsUnsupportedError_NegativeCases(t *testing.T) {
  t.Parallel()
  cases := []string{
    "",
    "temporary outage: service unavailable",                    // 'unavailable' is not an indicator in current logic
    "rate limit exceeded; please retry later",
    "one triggers other is not implemented",                    // lacks exact 'one-triggers' or other indicators
    "operation aborted by user",
    "unknown error occurred",
    "request canceled (context deadline exceeded)",
    "Feature flagged off",                                      // vague, no indicator
    "Broker message: feature coming soon",                      // no indicator
  }

  for i, msg := range cases {
    if containsUnsupportedError(msg) {
      t.Fatalf("case %d expected false for message: %q", i, msg)
    }
  }
}

func TestContainsUnsupportedError_CaseInsensitivityAndSpacing(t *testing.T) {
  t.Parallel()
  cases := map[string]bool{
    "UNSUPPORTED":                                  true,
    "Not Supported":                                 true,
    "not available":                                 true,
    "Invalid Order Type":                            true,
    "feature: OTO supported? no -> not supported":   true,
    "one-triggers-other is disabled":                true, // contains 'one-triggers'
    "OCO Order rejected":                            true,
    "unsupported? no keyword here actually":         true, // still true due to 'unsupported'
  }
  for msg, want := range cases {
    got := containsUnsupportedError(msg)
    if got != want {
      t.Fatalf("message %q: want %v, got %v", msg, want, got)
    }
  }
}

func TestContainsUnsupportedError_LongMultiLine(t *testing.T) {
  t.Parallel()
  msg := "error placing order\nDetails:\nThe requested order type is Not Available for your account.\nPlease contact support."
  if !containsUnsupportedError(msg) {
    t.Fatalf("expected true for multi-line message indicating not available")
  }
}

// Optional fuzz-like table to catch tricky substrings and ensure current behavior is stable.
// Note: we intentionally avoid cases that would be 'false positives' per human intent,
// and instead document current matching semantics (simple substring check).
func TestContainsUnsupportedError_Stability_Keywords(t *testing.T) {
  t.Parallel()
  keywords := []string{"unsupported", "not supported", "not available", "invalid order type", "oto", "one-triggers", "oco"}
  for _, kw := range keywords {
    msg := "prefix ... " + kw + " ... suffix"
    if !containsUnsupportedError(msg) {
      t.Fatalf("keyword %q should trigger true", kw)
    }
  }
}