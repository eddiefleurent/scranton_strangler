package models

import (
	"testing"
)

func TestStateMachine_RecoveredPositionTransition(t *testing.T) {
	sm := NewStateMachine()

	// Test direct transition from Idle to Open for recovered positions
	err := sm.Transition(StateOpen, ConditionRecoveredPosition)
	if err != nil {
		t.Fatalf("Failed to transition from Idle to Open with recovered position: %v", err)
	}

	if sm.GetCurrentState() != StateOpen {
		t.Errorf("Expected state to be Open, got %v", sm.GetCurrentState())
	}

	// Verify we can continue normal flow from Open state
	err = sm.Transition(StateFirstDown, ConditionStartManagement)
	if err != nil {
		t.Fatalf("Failed to transition from Open to FirstDown: %v", err)
	}

	if sm.GetCurrentState() != StateFirstDown {
		t.Errorf("Expected state to be FirstDown, got %v", sm.GetCurrentState())
	}
}

func TestStateMachine_InvalidRecoveredPositionTransition(t *testing.T) {
	sm := NewStateMachine()

	// First leave Idle by placing an order (Idle -> Submitted)
	err := sm.Transition(StateSubmitted, ConditionOrderPlaced)
	if err != nil {
		t.Fatalf("Failed to transition to Submitted: %v", err)
	}
	if sm.GetCurrentState() != StateSubmitted {
		t.Fatalf("Expected state to be Submitted after OrderPlaced, got %v", sm.GetCurrentState())
	}

	// Try to use recovered position transition from non-Idle state
	err = sm.Transition(StateOpen, ConditionRecoveredPosition)
	if err == nil {
		t.Error("Expected error when using recovered position transition from non-Idle state")
	}
	if sm.GetCurrentState() != StateSubmitted {
		t.Errorf("State mutated unexpectedly; expected Submitted, got %v", sm.GetCurrentState())
	}
}

func TestNewStateMachineFromState_RecoveredPosition(t *testing.T) {
	// Test creating a state machine directly in Open state (simulating recovered position)
	sm := NewStateMachineFromState(StateOpen)

	if sm.GetCurrentState() != StateOpen {
		t.Errorf("Expected state to be Open, got %v", sm.GetCurrentState())
	}

	// Verify we can transition normally from this state
	err := sm.Transition(StateFirstDown, ConditionStartManagement)
	if err != nil {
		t.Fatalf("Failed to transition from Open to FirstDown: %v", err)
	}

	if sm.GetCurrentState() != StateFirstDown {
		t.Errorf("Expected state to be FirstDown, got %v", sm.GetCurrentState())
	}
}