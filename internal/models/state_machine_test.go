package models

import (
	"testing"
	"time"
)

func TestStateMachine_BasicTransitions(t *testing.T) {
	sm := NewStateMachine()
	
	// Test initial state
	if sm.GetCurrentState() != StateIdle {
		t.Errorf("Initial state should be StateIdle, got %s", sm.GetCurrentState())
	}
	
	// Test valid transition: Idle -> Open
	err := sm.Transition(StateOpen, "position_filled")
	if err != nil {
		t.Errorf("Valid transition failed: %v", err)
	}
	
	if sm.GetCurrentState() != StateOpen {
		t.Errorf("State should be StateOpen, got %s", sm.GetCurrentState())
	}
	
	if sm.GetPreviousState() != StateIdle {
		t.Errorf("Previous state should be StateIdle, got %s", sm.GetPreviousState())
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	sm := NewStateMachine()
	
	// Test invalid transition: Idle -> FirstDown (skipping intermediate states)
	err := sm.Transition(StateFirstDown, "invalid")
	if err == nil {
		t.Error("Invalid transition should fail")
	}
	
	// State should remain unchanged after failed transition
	if sm.GetCurrentState() != StateIdle {
		t.Errorf("State should remain StateIdle after failed transition, got %s", sm.GetCurrentState())
	}
}

func TestStateMachine_FootballSystemFlow(t *testing.T) {
	sm := NewStateMachine()
	
	// Complete entry flow (simplified)
	transitions := []struct {
		to        PositionState
		condition string
	}{
		{StateOpen, "position_filled"},
		{StateFirstDown, "start_management"},
	}
	
	for _, tr := range transitions {
		err := sm.Transition(tr.to, tr.condition)
		if err != nil {
			t.Fatalf("Transition to %s failed: %v", tr.to, err)
		}
	}
	
	// Test football system progression
	if !sm.IsManagementState() {
		t.Error("Should be in management state")
	}
	
	if sm.GetManagementPhase() != 1 {
		t.Errorf("Should be in phase 1 (First Down), got %d", sm.GetManagementPhase())
	}
	
	// Progress through football phases
	footballTransitions := []struct {
		to        PositionState
		condition string
		phase     int
	}{
		{StateSecondDown, "strike_challenged", 2},
		{StateThirdDown, "strike_breached", 3},
		{StateFourthDown, "adjustment_failed", 4},
	}
	
	for _, tr := range footballTransitions {
		err := sm.Transition(tr.to, tr.condition)
		if err != nil {
			t.Fatalf("Football transition to %s failed: %v", tr.to, err)
		}
		
		if sm.GetManagementPhase() != tr.phase {
			t.Errorf("Should be in phase %d, got %d", tr.phase, sm.GetManagementPhase())
		}
	}
}

func TestStateMachine_AdjustmentLimits(t *testing.T) {
	sm := NewStateMachine()
	
	// Setup position in adjustment-capable state (simplified)
	setupTransitions := []struct {
		to        PositionState
		condition string
	}{
		{StateOpen, "position_filled"},
		{StateFirstDown, "start_management"},
		{StateSecondDown, "strike_challenged"},
	}
	
	for _, tr := range setupTransitions {
		sm.Transition(tr.to, tr.condition)
	}
	
	// Test adjustment limits (max 3)
	for i := 0; i < 3; i++ {
		// Go to adjusting state
		err := sm.Transition(StateAdjusting, "roll_untested")
		if err != nil {
			t.Fatalf("Adjustment %d failed: %v", i+1, err)
		}
		
		if !sm.CanAdjust() && i < 2 {
			t.Errorf("Should still be able to adjust after %d adjustments", i+1)
		}
		
		// Return to management state
		sm.Transition(StateFirstDown, "adjustment_complete")
		sm.Transition(StateSecondDown, "strike_challenged")
	}
	
	// Fourth adjustment should fail
	err := sm.Transition(StateAdjusting, "roll_untested")
	if err == nil {
		t.Error("Fourth adjustment should be rejected")
	}
	
	if sm.CanAdjust() {
		t.Error("Should not be able to adjust after 3 attempts")
	}
}

func TestStateMachine_TimeRollLimits(t *testing.T) {
	sm := NewStateMachine()
	
	// Setup position in critical state for time roll (simplified)
	setupTransitions := []struct {
		to        PositionState
		condition string
	}{
		{StateOpen, "position_filled"},
		{StateFirstDown, "start_management"},
		{StateSecondDown, "strike_challenged"},
		{StateThirdDown, "strike_breached"},
		{StateFourthDown, "adjustment_failed"},
	}
	
	for _, tr := range setupTransitions {
		sm.Transition(tr.to, tr.condition)
	}
	
	// First time roll should succeed
	err := sm.Transition(StateRolling, "punt_decision")
	if err != nil {
		t.Fatalf("First time roll failed: %v", err)
	}
	
	// After first roll, we can't roll again (max 1)
	if sm.CanRoll() {
		t.Error("Should NOT be able to roll after first attempt (max 1 allowed)")
	}
	
	// Complete the roll and get back to critical state
	sm.Transition(StateFirstDown, "roll_complete")
	sm.Transition(StateSecondDown, "strike_challenged")
	sm.Transition(StateThirdDown, "strike_breached")
	sm.Transition(StateFourthDown, "adjustment_failed")
	
	// Second time roll should fail (max 1 allowed)
	err = sm.Transition(StateRolling, "punt_decision")
	if err == nil {
		t.Error("Second time roll should be rejected")
	}
}

func TestStateMachine_Reset(t *testing.T) {
	sm := NewStateMachine()
	
	// Make several transitions (simplified)
	sm.Transition(StateOpen, "position_filled")
	sm.Transition(StateFirstDown, "start_management")
	sm.Transition(StateSecondDown, "strike_challenged")
	sm.Transition(StateAdjusting, "roll_untested")
	
	// Verify state is not idle
	if sm.GetCurrentState() == StateIdle {
		t.Error("State should not be idle before reset")
	}
	
	if sm.GetTransitionCount(StateAdjusting) != 1 {
		t.Errorf("Should have 1 adjustment count before reset, got %d", sm.GetTransitionCount(StateAdjusting))
	}
	
	// Reset
	sm.Reset()
	
	// Verify reset worked
	if sm.GetCurrentState() != StateIdle {
		t.Errorf("State should be idle after reset, got %s", sm.GetCurrentState())
	}
	
	if sm.GetTransitionCount(StateAdjusting) != 0 {
		t.Error("Transition counts should be reset to zero")
	}
	
	if !sm.CanAdjust() || !sm.CanRoll() {
		t.Error("Should be able to adjust and roll after reset")
	}
}

func TestStateMachine_StateValidation(t *testing.T) {
	sm := NewStateMachine()
	
	// Test validation on fresh state machine
	err := sm.ValidateStateConsistency()
	if err != nil {
		t.Errorf("Fresh state machine should be valid: %v", err)
	}
	
	// Test validation after normal transitions
	sm.Transition(StateOpen, "position_filled")
	sm.Transition(StateFirstDown, "start_management")
	
	err = sm.ValidateStateConsistency()
	if err != nil {
		t.Errorf("Normal state machine should be valid: %v", err)
	}
}

func TestStateMachine_StateDescriptions(t *testing.T) {
	sm := NewStateMachine()
	
	testStates := []PositionState{
		StateIdle,
		StateOpen,
		StateFirstDown,
		StateSecondDown,
		StateThirdDown,
		StateFourthDown,
		StateAdjusting,
		StateRolling,
		StateClosed,
		StateError,
	}
	
	for _, state := range testStates {
		// Force state for testing (bypass validation)
		sm.currentState = state
		
		description := sm.GetStateDescription()
		if description == "" || description == "Unknown state" {
			t.Errorf("State %s should have a valid description, got: %s", state, description)
		}
	}
}

func TestPosition_StateMachineIntegration(t *testing.T) {
	// Test position creation with state machine
	expiration := time.Now().AddDate(0, 0, 45)
	pos := NewPosition("TEST-1", "SPY", 400.0, 420.0, expiration, 1)
	
	if pos.StateMachine == nil {
		t.Fatal("Position should have initialized state machine")
	}
	
	if pos.GetCurrentState() != StateIdle {
		t.Errorf("New position should start in StateIdle, got %s", pos.GetCurrentState())
	}
	
	// Test state transitions through position methods
	err := pos.TransitionState(StateOpen, "position_filled")
	if err != nil {
		t.Fatalf("Position state transition failed: %v", err)
	}
	
	if pos.GetCurrentState() != StateOpen {
		t.Errorf("Position state should be StateOpen, got %s", pos.GetCurrentState())
	}
	
}

func TestPosition_StateValidation(t *testing.T) {
	expiration := time.Now().AddDate(0, 0, 45)
	pos := NewPosition("TEST-1", "SPY", 400.0, 420.0, expiration, 1)
	
	// Test validation on new position
	err := pos.ValidateState()
	if err != nil {
		t.Errorf("New position should be valid: %v", err)
	}
	
	// Set up position with credit and entry date
	pos.CreditReceived = 3.50
	pos.EntryDate = time.Now()
	
	// Transition to management state (simplified)
	pos.TransitionState(StateOpen, "position_filled")
	pos.TransitionState(StateFirstDown, "start_management")
	
	err = pos.ValidateState()
	if err != nil {
		t.Errorf("Positioned state should be valid: %v", err)
	}
	
	// Test invalid state - positioned without credit
	pos.CreditReceived = 0
	err = pos.ValidateState()
	if err == nil {
		t.Error("Position in management state without credit should be invalid")
	}
}

func TestPosition_ManagementMethods(t *testing.T) {
	expiration := time.Now().AddDate(0, 0, 45)
	pos := NewPosition("TEST-1", "SPY", 400.0, 420.0, expiration, 1)
	
	// Initially not in management
	if pos.IsInManagement() {
		t.Error("New position should not be in management")
	}
	
	if pos.GetManagementPhase() != 0 {
		t.Errorf("New position should be in phase 0, got %d", pos.GetManagementPhase())
	}
	
	// Transition to management state (simplified)
	pos.TransitionState(StateOpen, "position_filled")
	pos.TransitionState(StateFirstDown, "start_management")
	
	if !pos.IsInManagement() {
		t.Error("Position should be in management after FirstDown")
	}
	
	if pos.GetManagementPhase() != 1 {
		t.Errorf("Should be in management phase 1, got %d", pos.GetManagementPhase())
	}
	
	// Test adjustment capabilities
	if !pos.CanAdjust() {
		t.Error("Fresh position should be able to adjust")
	}
	
	if !pos.CanRoll() {
		t.Error("Fresh position should be able to roll")
	}
}