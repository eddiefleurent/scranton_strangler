package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/models"
)

// TestInterface tests the storage interface with both implementations
func TestInterface(t *testing.T) {
	// Test with MockStorage
	t.Run("MockStorage", func(t *testing.T) {
		storage := NewMockStorage()
		testInterface(t, storage)
	})

	// Test with JSONStorage (using temporary file)
	t.Run("JSONStorage", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := fmt.Sprintf("%s/test_positions_%d.json", tmpDir, time.Now().UnixNano())

		storage, err := NewJSONStorage(tmpFile)
		if err != nil {
			t.Fatalf("Failed to create JSON storage: %v", err)
		}
		testInterface(t, storage)
	})
}

// testInterface runs common tests on any storage implementation
func testInterface(t *testing.T, storage Interface) {
	// Test initial state
	pos := storage.GetCurrentPosition()
	if pos != nil {
		t.Error("Expected no current position initially")
	}

	// Create a test position
	testPos := models.NewPosition(
		"test-123",
		"SPY",
		445.0,                        // put strike
		455.0,                        // call strike
		time.Now().AddDate(0, 0, 30), // 30 DTE
		1,                            // quantity
	)
	testPos.CreditReceived = 3.50
	testPos.EntryIV = 45.0
	testPos.EntrySpot = 450.0

	// Transition to open state
	err := testPos.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to transition position to open: %v", err)
	}
	err = testPos.TransitionState(models.StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Failed to transition position to open: %v", err)
	}

	// Test setting current position
	err = storage.SetCurrentPosition(testPos)
	if err != nil {
		t.Fatalf("Failed to set current position: %v", err)
	}

	// Test getting current position
	retrievedPos := storage.GetCurrentPosition()
	if retrievedPos == nil {
		t.Fatal("Expected current position, got nil")
	}
	if retrievedPos.ID != testPos.ID {
		t.Errorf("Expected position ID %s, got %s", testPos.ID, retrievedPos.ID)
	}
	if retrievedPos.GetCurrentState() != models.StateOpen {
		t.Errorf("Expected position state %s, got %s", models.StateOpen, retrievedPos.GetCurrentState())
	}
	// Mutate the returned copy; storage should be unaffected.
	retrievedPos.CurrentPnL = 999
	if storage.GetCurrentPosition().CurrentPnL == 999 {
		t.Errorf("GetCurrentPosition leaked internal state (mutation visible)")
	}

	// Test adding adjustment
	adjustment := models.Adjustment{
		Date:        time.Now(),
		Type:        "roll_call",
		OldStrike:   455.0,
		NewStrike:   460.0,
		Credit:      1.25,
		Description: "Rolled call side up due to upward pressure",
	}

	err = storage.AddAdjustment(adjustment)
	if err != nil {
		t.Fatalf("Failed to add adjustment: %v", err)
	}

	// Verify adjustment was added
	currentPos := storage.GetCurrentPosition()
	if len(currentPos.Adjustments) != 1 {
		t.Errorf("Expected 1 adjustment, got %d", len(currentPos.Adjustments))
	}
	if currentPos.Adjustments[0].Type != "roll_call" {
		t.Errorf("Expected adjustment type 'roll_call', got %s", currentPos.Adjustments[0].Type)
	}

	// Test closing position
	finalPnL := 1.75 // 50% profit
	err = storage.ClosePosition(finalPnL, "position_closed")
	if err != nil {
		t.Fatalf("Failed to close position: %v", err)
	}

	// Verify position is closed
	if storage.GetCurrentPosition() != nil {
		t.Error("Expected no current position after closing")
	}

	// Verify position in history
	history := storage.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 position in history, got %d", len(history))
	}
	if history[0].ID != testPos.ID {
		t.Errorf("Expected position ID %s in history, got %s", testPos.ID, history[0].ID)
	}
	if history[0].CurrentPnL != finalPnL {
		t.Errorf("Expected final P&L %f, got %f", finalPnL, history[0].CurrentPnL)
	}
	if history[0].GetCurrentState() != models.StateClosed {
		t.Errorf("Expected closed state in history, got %s", history[0].GetCurrentState())
	}

	// Test statistics
	stats := storage.GetStatistics()
	if stats.TotalTrades != 1 {
		t.Errorf("Expected 1 total trade, got %d", stats.TotalTrades)
	}
	if stats.WinningTrades != 1 {
		t.Errorf("Expected 1 winning trade, got %d", stats.WinningTrades)
	}
	if stats.TotalPnL != finalPnL {
		t.Errorf("Expected total P&L %f, got %f", finalPnL, stats.TotalPnL)
	}
}

// TestMockStorageSpecificFeatures tests mock-specific features
func TestMockStorageSpecificFeatures(t *testing.T) {
	mock := NewMockStorage()

	// Test error injection
	testErr := &MockError{"test save error"}
	mock.SetSaveError(testErr)

	err := mock.Save()
	if err != testErr {
		t.Errorf("Expected injected save error, got %v", err)
	}

	// Test call counting
	mock.SetSaveError(nil) // Reset error
	err = mock.Save()
	if err != nil {
		t.Errorf("Unexpected save error: %v", err)
	}
	err = mock.Save()
	if err != nil {
		t.Errorf("Unexpected save error: %v", err)
	}

	if mock.GetSaveCallCount() != 3 { // 2 new + 1 from error test
		t.Errorf("Expected 3 save calls, got %d", mock.GetSaveCallCount())
	}

	// Test manual data setup
	testDate := "2024-01-15"
	testPnL := 125.50
	mock.SetDailyPnL(testDate, testPnL)

	retrievedPnL := mock.GetDailyPnL(testDate)
	if retrievedPnL != testPnL {
		t.Errorf("Expected daily P&L %f, got %f", testPnL, retrievedPnL)
	}
}

// MockError is a simple error type for testing
type MockError struct {
	message string
}

func (e *MockError) Error() string {
	return e.message
}

// TestExitMetadataBackup ensures exit metadata is properly set during position closure
func TestExitMetadataBackup(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := fmt.Sprintf("%s/test_exit_metadata_%d.json", tmpDir, time.Now().UnixNano())

	storage, err := NewJSONStorage(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create JSON storage: %v", err)
	}

	// Create and set a test position
	testPos := models.NewPosition(
		"test-exit-123",
		"SPY",
		445.0,
		455.0,
		time.Now().AddDate(0, 0, 30),
		1,
	)
	testPos.CreditReceived = 3.50

	// Transition to open state (must go through submitted first)
	err = testPos.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to transition position to submitted: %v", err)
	}
	err = testPos.TransitionState(models.StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Failed to transition position to open: %v", err)
	}

	err = storage.SetCurrentPosition(testPos)
	if err != nil {
		t.Fatalf("Failed to set current position: %v", err)
	}

	// Clear exit metadata to test backup functionality
	testPos.ExitReason = ""
	testPos.ExitDate = time.Time{}
	err = storage.SetCurrentPosition(testPos)
	if err != nil {
		t.Fatalf("Failed to update position: %v", err)
	}

	// Close position with valid condition
	finalPnL := -1.25 // Loss to test edge case
	t0 := time.Now()
	err = storage.ClosePosition(finalPnL, "position_closed")
	if err != nil {
		t.Fatalf("Failed to close position: %v", err)
	}

	// Verify position is in history with correct exit metadata
	history := storage.GetHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 position in history, got %d", len(history))
	}

	closedPos := history[0]

	// Verify exit reason is set (should be the transition condition)
	expectedReason := "position_closed"
	if closedPos.ExitReason != expectedReason {
		t.Errorf("Expected exit reason '%s', got '%s'", expectedReason, closedPos.ExitReason)
	}

	// Verify exit date is set (should be recent)
	if closedPos.ExitDate.IsZero() {
		t.Error("Expected exit date to be set, but it was zero time")
	}

	// Verify exit date falls within [t0-2s, now+2s]
	now := time.Now()
	if closedPos.ExitDate.Before(t0.Add(-2*time.Second)) || closedPos.ExitDate.After(now.Add(2*time.Second)) {
		t.Errorf("Exit date out of expected window: %v (t0=%v now=%v)", closedPos.ExitDate, t0, now)
	}

	// Verify final P&L is recorded
	if closedPos.CurrentPnL != finalPnL {
		t.Errorf("Expected final P&L %f, got %f", finalPnL, closedPos.CurrentPnL)
	}
}

// TestInterfaceCompliance ensures all implementations satisfy the interface
func TestInterfaceCompliance(t *testing.T) {
	// Test that both implementations satisfy the interface
	var _ Interface = (*MockStorage)(nil)
	var _ Interface = (*JSONStorage)(nil)

	// Test factory function
	tmpFile := fmt.Sprintf("%s/factory.json", t.TempDir())
	storage, err := NewStorage(tmpFile)
	if err != nil {
		t.Fatalf("Factory function failed: %v", err)
	}

	// Ensure factory returns the interface
	_ = storage
}
