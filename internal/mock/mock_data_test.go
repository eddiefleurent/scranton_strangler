package mock

import (
	"testing"
	"time"
)

func TestDataProvider_GetOptionChain_InvalidExpiration(t *testing.T) {
	provider := NewDataProvider()

	// Test with invalid expiration format
	_, err := provider.GetOptionChain("SPY", "invalid-date", true)
	if err == nil {
		t.Error("Expected error for invalid expiration format, got nil")
	}

	// Test with past expiration (should not error but should handle gracefully)
	pastDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	options, err := provider.GetOptionChain("SPY", pastDate, true)
	if err != nil {
		t.Errorf("Unexpected error for past expiration: %v", err)
	}
	if len(options) == 0 {
		t.Error("Expected some options even for past expiration")
	}
}

func TestDataProvider_GenerateSamplePosition_ErrorHandling(t *testing.T) {
	provider := NewDataProvider()

	// This should not panic even if there are issues
	result := provider.GenerateSamplePosition()

	// Should return a map (even if empty)
	if result == nil {
		t.Error("GenerateSamplePosition should not return nil")
	}
}
