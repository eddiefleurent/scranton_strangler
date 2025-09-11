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
	positions := storage.GetCurrentPositions()
	if len(positions) != 0 {
		t.Error("Expected no current positions initially")
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

	// Transition to open state
	err := testPos.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to transition position to open: %v", err)
	}
	err = testPos.TransitionState(models.StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Failed to transition position to open: %v", err)
	}

	// Set position data after transitions (when position is Open)
	testPos.CreditReceived = 3.50
	testPos.EntryIV = 45.0
	testPos.EntrySpot = 450.0
	testPos.Quantity = 1 // Ensure Quantity is set to positive value for Open invariants

	// Test adding position
	err = storage.AddPosition(testPos)
	if err != nil {
		t.Fatalf("Failed to add position: %v", err)
	}

	// Test getting current positions
	positions = storage.GetCurrentPositions()
	if len(positions) != 1 {
		t.Fatalf("Expected 1 position, got %d", len(positions))
	}
	retrievedPos := &positions[0]
	if retrievedPos.ID != testPos.ID {
		t.Errorf("Expected position ID %s, got %s", testPos.ID, retrievedPos.ID)
	}
	if retrievedPos.GetCurrentState() != models.StateOpen {
		t.Errorf("Expected position state %s, got %s", models.StateOpen, retrievedPos.GetCurrentState())
	}
	
	// Test GetPositionByID
	posById, found := storage.GetPositionByID(testPos.ID)
	if !found {
		t.Fatal("Expected to find position by ID")
	}
	if posById.ID != testPos.ID {
		t.Errorf("Expected position ID %s, got %s", testPos.ID, posById.ID)
	}

	// Mutate the returned copy; storage should be unaffected
	retrievedPos.CurrentPnL = 999
	positions2 := storage.GetCurrentPositions()
	if len(positions2) != 1 || positions2[0].CurrentPnL == 999 {
		t.Error("Expected deep copy protection, but internal state was mutated")
	}

	// Test updating position with adjustment
	adjustment := models.Adjustment{
		Date:        time.Now(),
		Type:        models.AdjustmentRoll,
		OldStrike:   455.0,
		NewStrike:   460.0,
		Credit:      1.25,
		Description: "Rolled call side up due to upward pressure",
	}
	testPos.Adjustments = append(testPos.Adjustments, adjustment)
	err = storage.UpdatePosition(testPos)
	if err != nil {
		t.Fatalf("Failed to update position: %v", err)
	}

	// Verify adjustment was added
	currentPos, found := storage.GetPositionByID(testPos.ID)
	if !found {
		t.Fatal("Position disappeared after update")
	}
	if len(currentPos.Adjustments) != 1 {
		t.Errorf("Expected 1 adjustment, got %d", len(currentPos.Adjustments))
	}
	if currentPos.Adjustments[0].Type != models.AdjustmentRoll {
		t.Errorf("Expected adjustment type '%s', got %s", models.AdjustmentRoll, currentPos.Adjustments[0].Type)
	}

	// Test closing position
	finalPnL := 1.75 // 50% profit
	err = storage.ClosePositionByID(testPos.ID, finalPnL, "position_closed")
	if err != nil {
		t.Fatalf("Failed to close position: %v", err)
	}

	// Verify position is closed
	positions = storage.GetCurrentPositions()
	if len(positions) != 0 {
		t.Error("Expected no current positions after closing")
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
	// Verify deep copy (mutating caller's copy must not affect storage)
	history[0].CurrentPnL = 9999
	if storage.GetHistory()[0].CurrentPnL == 9999 {
		t.Errorf("GetHistory leaked internal state (mutation visible)")
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

	// Transition to open state (must go through submitted first)
	err = testPos.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to transition position to submitted: %v", err)
	}
	err = testPos.TransitionState(models.StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Failed to transition position to open: %v", err)
	}

	// Post-fill details (persisted once open)
	testPos.CreditReceived = 3.50
	testPos.Quantity = 1

	err = storage.AddPosition(testPos)
	if err != nil {
		t.Fatalf("Failed to add position: %v", err)
	}

	// Clear exit metadata to test backup functionality
	testPos.ExitReason = ""
	testPos.ExitDate = time.Time{}
	err = storage.UpdatePosition(testPos)
	if err != nil {
		t.Fatalf("Failed to update position: %v", err)
	}

	// Close position with valid condition
	finalPnL := -1.25 // Loss to test edge case
	t0 := time.Now()
	err = storage.ClosePositionByID(testPos.ID, finalPnL, "position_closed")
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

// TestClosePositionReasonMappings tests the explicit reason to condition mappings
func TestClosePositionReasonMappings(t *testing.T) {
	testCases := []struct {
		reason            string
		expectedCondition string
		description       string
		initialState      models.PositionState // State that allows the expected condition
	}{
		{"manual", models.ConditionForceClose, "manual close should map to force_close condition", models.StateError},
		{"force_close", models.ConditionForceClose, "force_close should map to force_close condition", models.StateRolling},
		{"hard_stop", models.ConditionHardStop, "hard_stop should map to hard_stop condition", models.StateThirdDown},
		{"stop_loss", models.ConditionHardStop, "stop_loss should map to hard_stop condition", models.StateAdjusting},
		{"profit_target", models.ConditionExitConditions, "profit_target should map to exit_conditions", models.StateFirstDown},
		{"time", models.ConditionExitConditions, "time should map to exit_conditions", models.StateSecondDown},
		{"emergency_exit", models.ConditionEmergencyExit, "emergency_exit should map to emergency_exit condition", models.StateFourthDown},
		{"escalate", models.ConditionEmergencyExit, "escalate should map to emergency_exit condition", models.StateFourthDown},
	}

	for _, tc := range testCases {
		t.Run(tc.reason, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := fmt.Sprintf("%s/test_reason_mapping_%s_%d.json", tmpDir, tc.reason, time.Now().UnixNano())

			storage, err := NewJSONStorage(tmpFile)
			if err != nil {
				t.Fatalf("Failed to create JSON storage: %v", err)
			}

			// Create and add a test position
			testPos := models.NewPosition(
				fmt.Sprintf("test-reason-%s", tc.reason),
				"SPY",
				445.0,
				455.0,
				time.Now().AddDate(0, 0, 30),
				1,
			)

			// Set up position to be in the state that allows the expected condition
			setupPositionForState(t, testPos, tc.initialState)

			err = storage.AddPosition(testPos)
			if err != nil {
				t.Fatalf("Failed to add position: %v", err)
			}

			// Close position with the specific reason
			finalPnL := 1.75
			err = storage.ClosePositionByID(testPos.ID, finalPnL, tc.reason)
			if err != nil {
				t.Fatalf("Failed to close position with reason '%s': %v", tc.reason, err)
			}

			// Verify position is in history with expected condition
			history := storage.GetHistory()
			if len(history) != 1 {
				t.Fatalf("Expected 1 position in history, got %d", len(history))
			}

			closedPos := history[0]
			if closedPos.ExitReason != tc.reason {
				t.Errorf("Expected exit reason '%s', got '%s'", tc.reason, closedPos.ExitReason)
			}

			// Check the state machine was transitioned with the correct condition
			// by verifying the position reached closed state
			if closedPos.GetCurrentState() != models.StateClosed {
				t.Errorf("Expected position to be in closed state, got %s", closedPos.GetCurrentState())
			}
		})
	}
}

// TestClosePositionDefaultStateMappings tests the default branch state to condition mappings
func TestClosePositionDefaultStateMappings(t *testing.T) {
	testCases := []struct {
		state             models.PositionState
		expectedCondition string
		description       string
	}{
		{models.StateOpen, models.ConditionPositionClosed, "StateOpen should map to position_closed condition"},
		{models.StateSubmitted, models.ConditionOrderTimeout, "StateSubmitted should map to order_timeout condition"},
		{models.StateFirstDown, models.ConditionExitConditions, "StateFirstDown should map to exit_conditions"},
		{models.StateSecondDown, models.ConditionExitConditions, "StateSecondDown should map to exit_conditions"},
		{models.StateThirdDown, models.ConditionHardStop, "StateThirdDown should map to hard_stop condition"},
		{models.StateFourthDown, models.ConditionEmergencyExit, "StateFourthDown should map to emergency_exit condition"},
		{models.StateError, models.ConditionForceClose, "StateError should map to force_close condition"},
		{models.StateAdjusting, models.ConditionHardStop, "StateAdjusting should map to hard_stop condition"},
		{models.StateRolling, models.ConditionForceClose, "StateRolling should map to force_close condition"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.state), func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := fmt.Sprintf("%s/test_state_mapping_%s_%d.json", tmpDir, tc.state, time.Now().UnixNano())

			storage, err := NewJSONStorage(tmpFile)
			if err != nil {
				t.Fatalf("Failed to create JSON storage: %v", err)
			}

			// Create a test position and set it to the desired state
			testPos := models.NewPosition(
				fmt.Sprintf("test-state-%s", tc.state),
				"SPY",
				445.0,
				455.0,
				time.Now().AddDate(0, 0, 30),
				1,
			)

			// Set up position to reach the target state
			setupPositionForState(t, testPos, tc.state)

			err = storage.AddPosition(testPos)
			if err != nil {
				t.Fatalf("Failed to add position: %v", err)
			}

			// Close position with a generic reason to trigger default mapping
			finalPnL := 0.50
			genericReason := "generic_close_reason"
			err = storage.ClosePositionByID(testPos.ID, finalPnL, genericReason)
			if err != nil {
				t.Fatalf("Failed to close position in state '%s': %v", tc.state, err)
			}

			// Verify position is in history
			history := storage.GetHistory()
			if len(history) != 1 {
				t.Fatalf("Expected 1 position in history, got %d", len(history))
			}

			closedPos := history[0]
			if closedPos.ExitReason != genericReason {
				t.Errorf("Expected exit reason '%s', got '%s'", genericReason, closedPos.ExitReason)
			}

			// Check the position reached closed state
			if closedPos.GetCurrentState() != models.StateClosed {
				t.Errorf("Expected position to be in closed state, got %s", closedPos.GetCurrentState())
			}
		})
	}
}

// TestClosePositionFallbackCondition tests the fallback condition for unknown states
func TestClosePositionFallbackCondition(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := fmt.Sprintf("%s/test_fallback_%d.json", tmpDir, time.Now().UnixNano())

	storage, err := NewJSONStorage(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create JSON storage: %v", err)
	}

	// Create a test position
	testPos := models.NewPosition(
		"test-fallback",
		"SPY",
		445.0,
		455.0,
		time.Now().AddDate(0, 0, 30),
		1,
	)

	// Set to first down state where ConditionExitConditions is valid for fallback
	setupPositionForState(t, testPos, models.StateFirstDown)

	err = storage.AddPosition(testPos)
	if err != nil {
		t.Fatalf("Failed to add position: %v", err)
	}

	// Close position with a generic reason to trigger default mapping
	finalPnL := 0.25
	genericReason := "unknown_state_close"
	err = storage.ClosePositionByID(testPos.ID, finalPnL, genericReason)
	if err != nil {
		t.Fatalf("Failed to close position with unknown state: %v", err)
	}

	// Verify position is in history
	history := storage.GetHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 position in history, got %d", len(history))
	}

	closedPos := history[0]
	if closedPos.ExitReason != genericReason {
		t.Errorf("Expected exit reason '%s', got '%s'", genericReason, closedPos.ExitReason)
	}

	// Check the position reached closed state (fallback should use exit_conditions)
	if closedPos.GetCurrentState() != models.StateClosed {
		t.Errorf("Expected position to be in closed state, got %s", closedPos.GetCurrentState())
	}
}

// setupPositionForState configures a position to be in the specified state
func setupPositionForState(t *testing.T, pos *models.Position, targetState models.PositionState) {
	// Start from idle and work our way to the target state
	switch targetState {
	case models.StateIdle:
		// Position starts in idle, nothing to do
		return
	case models.StateSubmitted:
		err := pos.TransitionState(models.StateSubmitted, models.ConditionOrderPlaced)
		if err != nil {
			t.Fatalf("Failed to transition to submitted: %v", err)
		}
	case models.StateOpen:
		err := pos.TransitionState(models.StateSubmitted, models.ConditionOrderPlaced)
		if err != nil {
			t.Fatalf("Failed to transition to submitted: %v", err)
		}
		err = pos.TransitionState(models.StateOpen, models.ConditionOrderFilled)
		if err != nil {
			t.Fatalf("Failed to transition to open: %v", err)
		}
		// Set required fields for open state
		pos.CreditReceived = 3.50
		pos.Quantity = 1
	case models.StateFirstDown:
		setupPositionForState(t, pos, models.StateOpen)
		err := pos.TransitionState(models.StateFirstDown, models.ConditionStartManagement)
		if err != nil {
			t.Fatalf("Failed to transition to first down: %v", err)
		}
	case models.StateSecondDown:
		setupPositionForState(t, pos, models.StateFirstDown)
		err := pos.TransitionState(models.StateSecondDown, models.ConditionStrikeChallenged)
		if err != nil {
			t.Fatalf("Failed to transition to second down: %v", err)
		}
	case models.StateThirdDown:
		setupPositionForState(t, pos, models.StateSecondDown)
		err := pos.TransitionState(models.StateThirdDown, models.ConditionStrikeBreached)
		if err != nil {
			t.Fatalf("Failed to transition to third down: %v", err)
		}
	case models.StateFourthDown:
		setupPositionForState(t, pos, models.StateThirdDown)
		err := pos.TransitionState(models.StateFourthDown, models.ConditionAdjustmentFailed)
		if err != nil {
			t.Fatalf("Failed to transition to fourth down: %v", err)
		}
	case models.StateError:
		err := pos.TransitionState(models.StateSubmitted, models.ConditionOrderPlaced)
		if err != nil {
			t.Fatalf("Failed to transition to submitted: %v", err)
		}
		err = pos.TransitionState(models.StateError, models.ConditionOrderFailed)
		if err != nil {
			t.Fatalf("Failed to transition to error: %v", err)
		}
	case models.StateAdjusting:
		setupPositionForState(t, pos, models.StateSecondDown)
		err := pos.TransitionState(models.StateAdjusting, models.ConditionRollUntested)
		if err != nil {
			t.Fatalf("Failed to transition to adjusting: %v", err)
		}
	case models.StateRolling:
		setupPositionForState(t, pos, models.StateFourthDown)
		err := pos.TransitionState(models.StateRolling, models.ConditionRollAsPunt)
		if err != nil {
			t.Fatalf("Failed to transition to rolling: %v", err)
		}
	default:
		t.Fatalf("Unknown target state: %s", targetState)
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
