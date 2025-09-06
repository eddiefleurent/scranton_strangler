package orders

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
)

func TestManager_TimeoutTransitionReasons(t *testing.T) {
	// Test the timeoutTransitionReason function for different states
	m := &Manager{}

	tests := []struct {
		currentState   models.PositionState
		expectedReason string
		description    string
	}{
		{models.StateAdjusting, "hard_stop", "StateAdjusting should use hard_stop"},
		{models.StateRolling, "force_close", "StateRolling should use force_close"},
		{models.StateFirstDown, "exit_conditions", "StateFirstDown should use exit_conditions"},
		{models.StateSecondDown, "exit_conditions", "StateSecondDown should use exit_conditions"},
		{models.StateThirdDown, "hard_stop", "StateThirdDown should use hard_stop"},
		{models.StateFourthDown, "emergency_exit", "StateFourthDown should use emergency_exit"},
		{models.StateError, "force_close", "StateError should use force_close"},
		{models.StateIdle, "force_close", "Unknown state should default to force_close"},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			result := m.timeoutTransitionReason(test.currentState)
			if result != test.expectedReason {
				t.Errorf("Expected %s for state %s, got %s", test.expectedReason, test.currentState, result)
			}
		})
	}
}

func TestManager_HandleOrderTimeout_EntryOrder(t *testing.T) {
	// Test entry order timeout (StateSubmitted -> StateClosed with "order_timeout")
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockStorage := storage.NewMockStorage()

	// Create a position in StateSubmitted (entry order)
	position := models.NewPosition("test-pos", "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)
	err := position.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to set up test position: %v", err)
	}

	if err := mockStorage.SetCurrentPosition(position); err != nil {
		t.Fatalf("Failed to set up test position in storage: %v", err)
	}

	m := NewManager(nil, mockStorage, logger, nil)

	// Simulate entry order timeout
	m.handleOrderTimeout("test-pos")

	// Verify position transitioned to closed with correct reason
	updatedPosition := mockStorage.GetCurrentPosition()
	if updatedPosition.GetCurrentState() != models.StateClosed {
		t.Errorf("Expected position to be closed, got %s", updatedPosition.GetCurrentState())
	}
}

func TestManager_HandleOrderTimeout_ExitOrderFromAdjusting(t *testing.T) {
	// Test exit order timeout from StateAdjusting -> StateClosed with "hard_stop"
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockStorage := storage.NewMockStorage()

	// Create a position and follow proper state transitions to StateAdjusting
	position := models.NewPosition("test-pos", "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)
	position.EntryOrderID = "123" // Set as if entry order was placed

	// Follow proper state flow: Idle -> Submitted -> Open -> FirstDown -> SecondDown -> Adjusting
	err := position.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to transition to submitted: %v", err)
	}
	err = position.TransitionState(models.StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Failed to transition to open: %v", err)
	}
	err = position.TransitionState(models.StateFirstDown, "start_management")
	if err != nil {
		t.Fatalf("Failed to transition to first down: %v", err)
	}
	err = position.TransitionState(models.StateSecondDown, "strike_challenged")
	if err != nil {
		t.Fatalf("Failed to transition to second down: %v", err)
	}
	err = position.TransitionState(models.StateAdjusting, "roll_untested")
	if err != nil {
		t.Fatalf("Failed to transition to adjusting: %v", err)
	}

	// Set up exit order scenario
	position.ExitOrderID = "456" // Exit order is active
	position.ExitReason = "stop_loss"

	if err := mockStorage.SetCurrentPosition(position); err != nil {
		t.Fatalf("Failed to set up test position in storage: %v", err)
	}

	m := NewManager(nil, mockStorage, logger, nil)

	// Simulate exit order timeout
	m.handleOrderTimeout("test-pos")

	// Verify position transitioned to closed with correct reason
	updatedPosition := mockStorage.GetCurrentPosition()
	if updatedPosition.GetCurrentState() != models.StateClosed {
		t.Errorf("Expected position to be closed, got %s", updatedPosition.GetCurrentState())
	}
}

func TestManager_ExitConditionFromReason(t *testing.T) {
	// Test the exitConditionFromReason mapping
	m := &Manager{}

	tests := []struct {
		exitReason string
		expected   string
	}{
		{"profit_target", "exit_conditions"},
		{"time", "exit_conditions"},
		{"manual", "exit_conditions"},
		{"escalate", "emergency_exit"},
		{"stop_loss", "hard_stop"},
		{"error", "hard_stop"},
		{"unknown", "exit_conditions"}, // default
	}

	for _, test := range tests {
		t.Run(test.exitReason, func(t *testing.T) {
			result := m.exitConditionFromReason(test.exitReason)
			if result != test.expected {
				t.Errorf("Expected %s for exit reason %s, got %s", test.expected, test.exitReason, result)
			}
		})
	}
}
