package models

import (
	"testing"
)

func TestStateMachine_RecoveredPositionTransition(t *testing.T) {
	sm := NewStateMachine()

	// Test direct transition from Idle to Open for recovered positions
	err := sm.Transition(StateOpen, ConditionRecoveredPosition)
	if err != nil {
		t.Errorf("Failed to transition from Idle to Open with recovered position: %v", err)
	}

	if sm.GetCurrentState() != StateOpen {
		t.Errorf("Expected state to be Open, got %s", sm.GetCurrentState())
	}

	// Verify we can continue normal flow from Open state
	err = sm.Transition(StateFirstDown, ConditionStartManagement)
	if err != nil {
		t.Errorf("Failed to transition from Open to FirstDown: %v", err)
	}

	if sm.GetCurrentState() != StateFirstDown {
		t.Errorf("Expected state to be FirstDown, got %s", sm.GetCurrentState())
	}
}

func TestStateMachine_InvalidRecoveredPositionTransition(t *testing.T) {
	sm := NewStateMachine()

	// First move to Open state normally
	err := sm.Transition(StateSubmitted, ConditionOrderPlaced)
	if err != nil {
		t.Fatalf("Failed to transition to Submitted: %v", err)
	}

	// Try to use recovered position transition from non-Idle state
	err = sm.Transition(StateOpen, ConditionRecoveredPosition)
	if err == nil {
		t.Error("Expected error when using recovered position transition from non-Idle state")
	}
}

func TestNewStateMachineFromState_RecoveredPosition(t *testing.T) {
	// Test creating a state machine directly in Open state (simulating recovered position)
	sm := NewStateMachineFromState(StateOpen)

	if sm.GetCurrentState() != StateOpen {
		t.Errorf("Expected state to be Open, got %s", sm.GetCurrentState())
	}

	// Verify we can transition normally from this state
	err := sm.Transition(StateFirstDown, ConditionStartManagement)
	if err != nil {
		t.Errorf("Failed to transition from Open to FirstDown: %v", err)
	}

	if sm.GetCurrentState() != StateFirstDown {
		t.Errorf("Expected state to be FirstDown, got %s", sm.GetCurrentState())
	}
}