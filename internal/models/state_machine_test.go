package models

import (
	"testing"
	"time"
)

// Test constants for repeated strings
const (
	emergencyExitMessage = "emergency exit: loss 142.9% >= 200% threshold"
)

func TestStateMachine_BasicTransitions(t *testing.T) {
	sm := NewStateMachine()

	// Test initial state
	if sm.GetCurrentState() != StateIdle {
		t.Errorf("Initial state should be StateIdle, got %s", sm.GetCurrentState())
	}

	// Test valid transition: Idle -> Submitted -> Open
	err := sm.Transition(StateSubmitted, "order_placed")
	if err != nil {
		t.Errorf("Valid transition failed: %v", err)
	}

	err = sm.Transition(StateOpen, "order_filled")
	if err != nil {
		t.Errorf("Valid transition failed: %v", err)
	}

	if sm.GetCurrentState() != StateOpen {
		t.Errorf("State should be StateOpen, got %s", sm.GetCurrentState())
	}

	if sm.GetPreviousState() != StateSubmitted {
		t.Errorf("Previous state should be StateSubmitted, got %s", sm.GetPreviousState())
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	sm := NewStateMachine()

	// Test invalid transition: Idle -> FourthDown (no direct path)
	err := sm.Transition(StateFourthDown, "invalid")
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
		{StateSubmitted, "order_placed"},
		{StateOpen, "order_filled"},
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
		{StateSubmitted, "order_placed"},
		{StateOpen, "order_filled"},
		{StateFirstDown, "start_management"},
		{StateSecondDown, "strike_challenged"},
	}

	for _, tr := range setupTransitions {
		err := sm.Transition(tr.to, tr.condition)
		if err != nil {
			t.Fatalf("Setup transition failed: %v", err)
		}
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
		_ = sm.Transition(StateFirstDown, "adjustment_complete") //nolint:errcheck // test setup
		_ = sm.Transition(StateSecondDown, "strike_challenged")  //nolint:errcheck // test setup
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
		{StateSubmitted, "order_placed"},
		{StateOpen, "order_filled"},
		{StateFirstDown, "start_management"},
		{StateSecondDown, "strike_challenged"},
		{StateThirdDown, "strike_breached"},
		{StateFourthDown, "adjustment_failed"},
	}

	for _, tr := range setupTransitions {
		err := sm.Transition(tr.to, tr.condition)
		if err != nil {
			t.Fatalf("Setup transition failed: %v", err)
		}
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
	err = sm.Transition(StateFirstDown, "roll_complete")
	if err != nil {
		t.Fatalf("Roll complete transition failed: %v", err)
	}
	err = sm.Transition(StateSecondDown, "strike_challenged")
	if err != nil {
		t.Fatalf("Strike challenged transition failed: %v", err)
	}
	err = sm.Transition(StateThirdDown, "strike_breached")
	if err != nil {
		t.Fatalf("Strike breached transition failed: %v", err)
	}
	err = sm.Transition(StateFourthDown, "adjustment_failed")
	if err != nil {
		t.Fatalf("Adjustment failed transition failed: %v", err)
	}

	// Second time roll should fail (max 1 allowed)
	err = sm.Transition(StateRolling, "punt_decision")
	if err == nil {
		t.Error("Second time roll should be rejected")
	}
}

func TestStateMachine_Reset(t *testing.T) {
	sm := NewStateMachine()

	// Make several transitions (simplified)
	_ = sm.Transition(StateSubmitted, "order_placed")
	_ = sm.Transition(StateOpen, "order_filled")
	_ = sm.Transition(StateFirstDown, "start_management")
	_ = sm.Transition(StateSecondDown, "strike_challenged")
	_ = sm.Transition(StateAdjusting, "roll_untested")

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
	err = sm.Transition(StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Transition to Submitted failed: %v", err)
	}
	err = sm.Transition(StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Transition to Open failed: %v", err)
	}
	err = sm.Transition(StateFirstDown, "start_management")
	if err != nil {
		t.Fatalf("Transition to FirstDown failed: %v", err)
	}

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
	err := pos.TransitionState(StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Position state transition failed: %v", err)
	}

	err = pos.TransitionState(StateOpen, "order_filled")
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

	// Set up position with credit
	pos.CreditReceived = 3.50

	// Transition to management state (simplified)
	_ = pos.TransitionState(StateSubmitted, "order_placed")
	_ = pos.TransitionState(StateOpen, "order_filled")
	_ = pos.TransitionState(StateFirstDown, "start_management")

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
	_ = pos.TransitionState(StateSubmitted, "order_placed")
	_ = pos.TransitionState(StateOpen, "order_filled")
	_ = pos.TransitionState(StateFirstDown, "start_management")

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

// Test emergency exit conditions at 200% loss threshold
func TestStateMachine_EmergencyExit_200PercentLoss(t *testing.T) {
	sm := NewStateMachine()

	// Test below 200% loss - no emergency exit
	shouldExit, reason := sm.ShouldEmergencyExit(3.50, -6.50, 30) // 185.7% loss
	if shouldExit {
		t.Errorf("Should not emergency exit at 185.7%% loss, but got: %s", reason)
	}

	// Test exactly at 200% loss - should trigger
	shouldExit, reason = sm.ShouldEmergencyExit(3.50, -7.00, 30) // 200% loss
	if !shouldExit {
		t.Error("Should emergency exit at 200% loss")
	}
	if reason != "emergency exit: loss 200.0% >= 200% threshold" {
		t.Errorf("Expected specific loss message, got: %s", reason)
	}

	// Test above 200% loss - should trigger
	shouldExit, reason = sm.ShouldEmergencyExit(3.50, -8.00, 30) // 228.6% loss
	if !shouldExit {
		t.Error("Should emergency exit at 228.6% loss")
	}
}

// Test Fourth Down Option A (Inverted Strangle) 5-day time limit
func TestStateMachine_EmergencyExit_OptionA_TimeLimit(t *testing.T) {
	sm := NewStateMachine()
	sm.currentState = StateFourthDown
	sm.SetFourthDownOption(OptionA)

	// Manipulate time to simulate passage of days
	sm.fourthDownStartTime = time.Now().Add(-4 * 24 * time.Hour) // 4 days ago

	// Test within 5-day limit - no emergency exit
	shouldExit, reason := sm.ShouldEmergencyExit(3.50, -5.00, 30) // 142.9% loss, 4 days
	if shouldExit && reason != emergencyExitMessage {
		t.Errorf("Should not emergency exit due to time at 4 days, but got: %s", reason)
	}

	// Test exactly at 5-day limit - no exit yet (>5 days required)
	sm.fourthDownStartTime = time.Now().Add(-5 * 24 * time.Hour) // exactly 5 days ago
	shouldExit, reason = sm.ShouldEmergencyExit(3.50, -5.00, 30) // 142.9% loss, 5 days
	if shouldExit && reason != emergencyExitMessage {
		t.Errorf("Should not emergency exit at exactly 5 days, but got: %s", reason)
	}

	// Test beyond 5-day limit - should trigger
	sm.fourthDownStartTime = time.Now().Add(-6 * 24 * time.Hour) // 6 days ago
	shouldExit, reason = sm.ShouldEmergencyExit(3.50, -5.00, 30) // 142.9% loss, 6 days
	if !shouldExit {
		t.Error("Should emergency exit after 6 days for Option A")
	}
	if !contains(reason, "Option A exceeded 5-day limit") {
		t.Errorf("Expected Option A time limit message, got: %s", reason)
	}
}

// Test Fourth Down Option B (Hold Straddle) 3-day time limit
func TestStateMachine_EmergencyExit_OptionB_TimeLimit(t *testing.T) {
	sm := NewStateMachine()
	sm.currentState = StateFourthDown
	sm.SetFourthDownOption(OptionB)

	// Test within 3-day limit - no emergency exit
	sm.fourthDownStartTime = time.Now().Add(-2 * 24 * time.Hour)  // 2 days ago
	shouldExit, reason := sm.ShouldEmergencyExit(3.50, -5.00, 30) // 142.9% loss, 2 days
	if shouldExit && reason != emergencyExitMessage {
		t.Errorf("Should not emergency exit due to time at 2 days, but got: %s", reason)
	}

	// Test beyond 3-day limit - should trigger
	sm.fourthDownStartTime = time.Now().Add(-4 * 24 * time.Hour) // 4 days ago
	shouldExit, reason = sm.ShouldEmergencyExit(3.50, -5.00, 30) // 142.9% loss, 4 days
	if !shouldExit {
		t.Error("Should emergency exit after 4 days for Option B")
	}
	if !contains(reason, "Option B exceeded 3-day limit") {
		t.Errorf("Expected Option B time limit message, got: %s", reason)
	}
}

// Test Fourth Down Option C (Punt) 21 DTE limit
func TestStateMachine_EmergencyExit_OptionC_DTELimit(t *testing.T) {
	sm := NewStateMachine()
	sm.currentState = StateFourthDown
	sm.SetFourthDownOption(OptionC)
	sm.fourthDownStartTime = time.Now().Add(-1 * 24 * time.Hour) // 1 day ago

	// Test above 21 DTE - no emergency exit
	shouldExit, reason := sm.ShouldEmergencyExit(3.50, -5.00, 25) // 142.9% loss, 25 DTE
	if shouldExit && reason != emergencyExitMessage {
		t.Errorf("Should not emergency exit at 25 DTE, but got: %s", reason)
	}

	// Test exactly at 21 DTE - should trigger
	shouldExit, reason = sm.ShouldEmergencyExit(3.50, -5.00, 21) // 142.9% loss, 21 DTE
	if !shouldExit {
		t.Error("Should emergency exit at 21 DTE for Option C")
	}
	if !contains(reason, "Option C reached 21 DTE limit") {
		t.Errorf("Expected Option C DTE limit message, got: %s", reason)
	}

	// Test below 21 DTE - should trigger
	shouldExit, reason = sm.ShouldEmergencyExit(3.50, -5.00, 15) // 142.9% loss, 15 DTE
	if !shouldExit {
		t.Error("Should emergency exit at 15 DTE for Option C")
	}
}

// Test punt functionality and single-punt enforcement
func TestStateMachine_Punt_SingleUse(t *testing.T) {
	sm := NewStateMachine()

	// Initially should be able to punt
	if !sm.CanPunt() {
		t.Error("Fresh state machine should allow punt")
	}

	// Execute first punt
	err := sm.ExecutePunt()
	if err != nil {
		t.Fatalf("First punt should succeed: %v", err)
	}

	// After first punt, should not be able to punt again
	if sm.CanPunt() {
		t.Error("Should not be able to punt after first use (max 1 per trade)")
	}

	// Second punt attempt should fail
	err = sm.ExecutePunt()
	if err == nil {
		t.Error("Second punt should fail - only 1 allowed per original trade")
	}

	// Test punt counter
	if sm.puntCount != 1 {
		t.Errorf("Punt count should be 1, got %d", sm.puntCount)
	}
}

// Test Position-level emergency exit integration
func TestPosition_EmergencyExit_Integration(t *testing.T) {
	expiration := time.Now().AddDate(0, 0, 30) // 30 DTE
	pos := NewPosition("TEST-1", "SPY", 400.0, 420.0, expiration, 1)

	pos.CreditReceived = 3.50
	pos.CurrentPnL = -5.00 // 142.9% loss

	// Below 200% loss - no emergency exit
	shouldExit, reason := pos.ShouldEmergencyExit()
	if shouldExit {
		t.Errorf("Should not emergency exit at 142.9%% loss, but got: %s", reason)
	}

	// At 200% loss - should trigger
	pos.CurrentPnL = -7.00 // 200% loss
	shouldExit, reason = pos.ShouldEmergencyExit()
	if !shouldExit {
		t.Error("Should emergency exit at 200% loss")
	}
}

// Test Position-level Fourth Down option management
func TestPosition_FourthDownOptions(t *testing.T) {
	expiration := time.Now().AddDate(0, 0, 30)
	pos := NewPosition("TEST-1", "SPY", 400.0, 420.0, expiration, 1)

	// Test initial state
	if pos.GetFourthDownOption() != "" {
		t.Error("New position should not have Fourth Down option set")
	}

	// Test setting Option A
	pos.SetFourthDownOption(OptionA)
	if pos.GetFourthDownOption() != OptionA {
		t.Errorf("Expected Option A, got %s", pos.GetFourthDownOption())
	}

	// Test punt capability
	if !pos.CanPunt() {
		t.Error("Fresh position should allow punt")
	}

	// Execute punt
	err := pos.ExecutePunt()
	if err != nil {
		t.Fatalf("Punt should succeed: %v", err)
	}

	// After punt, should not be able to punt again
	if pos.CanPunt() {
		t.Error("Should not be able to punt after first use")
	}
}

// Test state machine reset includes new Fourth Down fields
func TestStateMachine_Reset_FourthDownFields(t *testing.T) {
	sm := NewStateMachine()

	// Set up Fourth Down state
	sm.currentState = StateFourthDown
	sm.SetFourthDownOption(OptionA)
	_ = sm.ExecutePunt()

	// Verify setup
	if sm.GetFourthDownOption() != OptionA {
		t.Error("Fourth Down option should be set before reset")
	}
	if sm.CanPunt() {
		t.Error("Should not be able to punt after using it")
	}

	// Reset state machine
	sm.Reset()

	// Verify Fourth Down fields are reset
	if sm.GetFourthDownOption() != "" {
		t.Errorf("Fourth Down option should be empty after reset, got %s", sm.GetFourthDownOption())
	}
	if !sm.CanPunt() {
		t.Error("Should be able to punt after reset")
	}
	if sm.puntCount != 0 {
		t.Errorf("Punt count should be 0 after reset, got %d", sm.puntCount)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsRecursive(s, substr, 0))
}

func containsRecursive(s, substr string, start int) bool {
	if start+len(substr) > len(s) {
		return false
	}
	if s[start:start+len(substr)] == substr {
		return true
	}
	return containsRecursive(s, substr, start+1)
}
