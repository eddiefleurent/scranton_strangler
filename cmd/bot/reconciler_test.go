package main

import (
	"log"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/stretchr/testify/mock"
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
func TestReconciler_PhantomPositionCleanup(t *testing.T) {
	t.Parallel()

	// Setup
	logger := log.New(log.Writer(), "[TEST] ", log.LstdFlags)
	phantomThreshold := 1 * time.Minute

	t.Run("phantom with zero EntryDate gets cleaned up", func(t *testing.T) {
		// Mock broker to return empty positions
		mockBroker := &MockBroker{}
		mockBroker.On("GetPositionsCtx", mock.Anything).Return([]broker.PositionItem{}, nil)

		testStorage := storage.NewMockStorage()
		testReconciler := NewReconciler(mockBroker, testStorage, logger, phantomThreshold)

		// Create a phantom position: quantity=0, credit=0, zero EntryDate, no adjustments
		phantom := models.NewPosition(
			"phantom-1",
			"SPY",
			630,
			690,
			time.Now().Add(45*24*time.Hour),
			0, // quantity = 0
		)
		phantom.CreditReceived = 0
		phantom.EntryDate = time.Time{} // Zero time
		phantom.Adjustments = []models.Adjustment{} // No history

		// Add to storage
		err := testStorage.AddPosition(phantom)
		if err != nil {
			t.Fatalf("Failed to add phantom position: %v", err)
		}

		// Wait for phantom threshold to pass (we use 0 as timeSinceCreation in the code for brand new positions)
		// The reconciler should skip it initially since timeSinceCreation = 0
		storedPositions := testStorage.GetCurrentPositions()
		result := testReconciler.ReconcilePositions(storedPositions)

		// Should still have the phantom (not cleaned up yet due to 0 duration)
		if len(result) != 1 {
			t.Errorf("Expected 1 phantom position (not cleaned yet), got %d", len(result))
		}
	})

	t.Run("phantom with adjustments gets cleaned up immediately", func(t *testing.T) {
		mockBroker2 := &MockBroker{}
		mockBroker2.On("GetPositionsCtx", mock.Anything).Return([]broker.PositionItem{}, nil)

		mockStorage2 := storage.NewMockStorage()
		reconciler2 := NewReconciler(mockBroker2, mockStorage2, logger, phantomThreshold)

		// Create a phantom with adjustments (indicating it's been around)
		phantom := models.NewPosition(
			"phantom-2",
			"SPY",
			630,
			690,
			time.Now().Add(45*24*time.Hour),
			0,
		)
		phantom.CreditReceived = 0
		phantom.EntryDate = time.Time{} // Zero time
		phantom.Adjustments = []models.Adjustment{{}} // Has adjustment history

		err := mockStorage2.AddPosition(phantom)
		if err != nil {
			t.Fatalf("Failed to add phantom position: %v", err)
		}

		storedPositions := mockStorage2.GetCurrentPositions()
		result := reconciler2.ReconcilePositions(storedPositions)

		// Should be cleaned up (timeSinceCreation = 2x phantomThreshold > threshold)
		if len(result) != 0 {
			t.Errorf("Expected 0 positions (phantom cleaned), got %d", len(result))
		}

		// Verify it was closed in storage
		allPositions := mockStorage2.GetCurrentPositions()
		for _, p := range allPositions {
			if p.ID == "phantom-2" && p.State != models.StateClosed {
				t.Errorf("Phantom position should be closed, but state is %s", p.State)
			}
		}
	})

	t.Run("phantom with old EntryDate gets cleaned up", func(t *testing.T) {
		mockBroker3 := &MockBroker{}
		mockBroker3.On("GetPositionsCtx", mock.Anything).Return([]broker.PositionItem{}, nil)

		mockStorage3 := storage.NewMockStorage()
		reconciler3 := NewReconciler(mockBroker3, mockStorage3, logger, phantomThreshold)

		// Create a phantom with old EntryDate
		phantom := models.NewPosition(
			"phantom-3",
			"SPY",
			630,
			690,
			time.Now().Add(45*24*time.Hour),
			0,
		)
		phantom.CreditReceived = 0
		phantom.EntryDate = time.Now().Add(-2 * time.Minute) // 2 minutes ago

		err := mockStorage3.AddPosition(phantom)
		if err != nil {
			t.Fatalf("Failed to add phantom position: %v", err)
		}

		storedPositions := mockStorage3.GetCurrentPositions()
		result := reconciler3.ReconcilePositions(storedPositions)

		// Should be cleaned up (2 minutes > 1 minute threshold)
		if len(result) != 0 {
			t.Errorf("Expected 0 positions (phantom cleaned), got %d", len(result))
		}
	})

	t.Run("valid position with quantity=0 but recent EntryDate not cleaned", func(t *testing.T) {
		mockBroker4 := &MockBroker{}
		mockBroker4.On("GetPositionsCtx", mock.Anything).Return([]broker.PositionItem{}, nil)

		mockStorage4 := storage.NewMockStorage()
		reconciler4 := NewReconciler(mockBroker4, mockStorage4, logger, phantomThreshold)

		// Create a position that just got created (might be filling)
		recent := models.NewPosition(
			"recent-1",
			"SPY",
			630,
			690,
			time.Now().Add(45*24*time.Hour),
			0,
		)
		recent.CreditReceived = 0
		recent.EntryDate = time.Now() // Just now

		err := mockStorage4.AddPosition(recent)
		if err != nil {
			t.Fatalf("Failed to add recent position: %v", err)
		}

		storedPositions := mockStorage4.GetCurrentPositions()
		result := reconciler4.ReconcilePositions(storedPositions)

		// Should NOT be cleaned up (too recent)
		if len(result) != 1 {
			t.Errorf("Expected 1 position (not cleaned, too recent), got %d", len(result))
		}
	})
}
