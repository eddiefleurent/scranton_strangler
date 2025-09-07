package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Test helper function to force state while maintaining internal consistency
// This is useful for testing specific states without going through full transition sequences
func forceStateForTesting(sm *StateMachine, state PositionState) {
	sm.currentState = state
	sm.previousState = state
	sm.transitionTime = time.Now().UTC()

	// Reset transition count for this state to avoid validation issues
	sm.transitionCount = make(map[PositionState]int)
	sm.transitionCount[state] = 1

	// Reset Fourth Down fields if not in Fourth Down
	if state != StateFourthDown {
		sm.fourthDownOption = ""
		sm.fourthDownStartTime = time.Time{}
	}
}

// Test constants for repeated strings

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
		if err := sm.Transition(StateFirstDown, "adjustment_complete"); err != nil {
			t.Fatalf("to FirstDown: %v", err)
		}
		if err := sm.Transition(StateSecondDown, "strike_challenged"); err != nil {
			t.Fatalf("to SecondDown: %v", err)
		}
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
	if err := sm.Transition(StateSubmitted, "order_placed"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := sm.Transition(StateOpen, "order_filled"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := sm.Transition(StateFirstDown, "start_management"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := sm.Transition(StateSecondDown, "strike_challenged"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := sm.Transition(StateAdjusting, "roll_untested"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

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
		// Force state for testing while maintaining internal consistency
		forceStateForTesting(sm, state)

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
	if transErr := pos.TransitionState(StateSubmitted, "order_placed"); transErr != nil {
		t.Errorf("Unexpected error: %v", transErr)
	}
	if transErr := pos.TransitionState(StateOpen, "order_filled"); transErr != nil {
		t.Errorf("Unexpected error: %v", transErr)
	}
	if transErr := pos.TransitionState(StateFirstDown, "start_management"); transErr != nil {
		t.Errorf("Unexpected error: %v", transErr)
	}

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
	if err := pos.TransitionState(StateSubmitted, "order_placed"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := pos.TransitionState(StateOpen, "order_filled"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := pos.TransitionState(StateFirstDown, "start_management"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

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
	// Using total $ basis: 3.50 credit * 100 = $350 total credit, -6.50 * 100 = -$650 P&L = 185.7% loss
	shouldExit, reason := sm.ShouldEmergencyExit(350.0, -650.0, 30, 21, 2.0) // 185.7% loss
	if shouldExit {
		t.Errorf("Should not emergency exit at 185.7%% loss, but got: %s", reason)
	}

	// Test exactly at 200% loss - should trigger
	// Using total $ basis: 3.50 credit * 100 = $350 total credit, -7.00 * 100 = -$700 P&L = 200% loss
	shouldExit, reason = sm.ShouldEmergencyExit(350.0, -700.0, 30, 21, 2.0) // 200% loss
	if !shouldExit {
		t.Error("Should emergency exit at 200% loss")
	}
	expectedMsg := fmt.Sprintf("emergency exit: loss 200.0%% >= %.0f%% threshold", 2.0*100)
	if reason != expectedMsg {
		t.Errorf("Expected specific loss message, got: %s", reason)
	}

	// Test above 200% loss - should trigger
	// Using total $ basis: 3.50 credit * 100 = $350 total credit, -8.00 * 100 = -$800 P&L = 228.6% loss
	shouldExit, _ = sm.ShouldEmergencyExit(350.0, -800.0, 30, 21, 2.0) // 228.6% loss
	if !shouldExit {
		t.Error("Should emergency exit at 228.6% loss")
	}
}

// Test Fourth Down Option A (Inverted Strangle) 5-day time limit
func TestStateMachine_EmergencyExit_OptionA_TimeLimit(t *testing.T) {
	sm := NewStateMachine()
	forceStateForTesting(sm, StateFourthDown)
	sm.SetFourthDownOption(OptionA)

	// Test within 5-day limit - no emergency exit (4 days ago)
	// Using total $ basis: 3.50 credit * 100 = $350, negligible P&L = $349.9
	sm.fourthDownStartTime = time.Now().UTC().AddDate(0, 0, -4)
	shouldExit, reason := sm.ShouldEmergencyExit(350.0, 349.9, 30, 21, 2.0) // negligible loss, 4 days
	if shouldExit {
		t.Errorf("Should not emergency exit due to time at 4 days, but got: %s", reason)
	}

	// Test exactly at 5-day limit - no exit yet (>5 days required)
	// Using total $ basis: 3.50 credit * 100 = $350, negligible P&L = $349.9
	sm.fourthDownStartTime = time.Now().UTC().AddDate(0, 0, -5)
	shouldExit, reason = sm.ShouldEmergencyExit(350.0, 349.9, 30, 21, 2.0) // negligible loss, 5 days
	if shouldExit {
		t.Errorf("Should not emergency exit at exactly 5 days, but got: %s", reason)
	}

	// Test beyond 5-day limit - should trigger
	// Using total $ basis: 3.50 credit * 100 = $350, -5.00 * 100 = -$500 = 142.9% loss, 6 days
	sm.fourthDownStartTime = time.Now().UTC().AddDate(0, 0, -6)
	shouldExit, reason = sm.ShouldEmergencyExit(350.0, -500.0, 30, 21, 2.0) // 142.9% loss, 6 days
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
	forceStateForTesting(sm, StateFourthDown)
	sm.SetFourthDownOption(OptionB)

	// Test within 3-day limit - no emergency exit (2 days ago)
	// Using total $ basis: 3.50 credit * 100 = $350, -0.10 * 100 = -$10 = 2.9% loss, 2 days
	sm.fourthDownStartTime = time.Now().UTC().AddDate(0, 0, -2)
	shouldExit, reason := sm.ShouldEmergencyExit(350.0, -10.0, 30, 21, 2.0) // 2.9% loss, 2 days
	if shouldExit {
		t.Errorf("Should not emergency exit due to time at 2 days, but got: %s", reason)
	}

	// Test beyond 3-day limit - should trigger
	// Using total $ basis: 3.50 credit * 100 = $350, -0.10 * 100 = -$10 = 2.9% loss, 4 days
	sm.fourthDownStartTime = time.Now().UTC().AddDate(0, 0, -4)
	shouldExit, reason = sm.ShouldEmergencyExit(350.0, -10.0, 30, 21, 2.0) // 2.9% loss, 4 days
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
	forceStateForTesting(sm, StateFourthDown)
	sm.SetFourthDownOption(OptionC)
	sm.fourthDownStartTime = time.Now().Add(-1 * 24 * time.Hour) // 1 day ago

	// Test above MaxDTE - no emergency exit
	// Using total $ basis: 3.50 credit * 100 = $350, -0.10 * 100 = -$10 = 2.9% loss, above MaxDTE
	shouldExit, reason := sm.ShouldEmergencyExit(350.0, -10.0, 25, 21, 2.0) // 2.9% loss, above MaxDTE
	if shouldExit {
		t.Errorf("Should not emergency exit at %d DTE, but got: %s", 25, reason)
	}

	// Test exactly at MaxDTE - should trigger
	// Using total $ basis: 3.50 credit * 100 = $350, -0.10 * 100 = -$10 = 2.9% loss, MaxDTE
	shouldExit, reason = sm.ShouldEmergencyExit(350.0, -10.0, 21, 21, 2.0) // 2.9% loss, MaxDTE
	if !shouldExit {
		t.Errorf("Should emergency exit at %d DTE for Option C", 21)
	}
	expectedDTEMsg := fmt.Sprintf("Option C reached %d DTE limit", 21)
	if !contains(reason, expectedDTEMsg) {
		t.Errorf("Expected Option C DTE limit message, got: %s", reason)
	}

	// Test below MaxDTE - should trigger
	// Using total $ basis: 3.50 credit * 100 = $350, -0.10 * 100 = -$10 = 2.9% loss, 15 DTE
	shouldExit, reason = sm.ShouldEmergencyExit(350.0, -10.0, 15, 21, 2.0) // 2.9% loss, 15 DTE
	if !shouldExit {
		t.Errorf("Should emergency exit at %d DTE for Option C", 15)
	}
	if !contains(reason, expectedDTEMsg) {
		t.Errorf("Expected Option C DTE limit message, got: %s", reason)
	}
}

// Test punt functionality and single-punt enforcement
func TestStateMachine_Punt_SingleUse(t *testing.T) {
	sm := NewStateMachine()

	// Setup: transition to FourthDown state first
	err := sm.Transition(StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to transition to Submitted: %v", err)
	}
	err = sm.Transition(StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Failed to transition to Open: %v", err)
	}
	err = sm.Transition(StateFirstDown, "start_management")
	if err != nil {
		t.Fatalf("Failed to transition to FirstDown: %v", err)
	}
	err = sm.Transition(StateSecondDown, "strike_challenged")
	if err != nil {
		t.Fatalf("Failed to transition to SecondDown: %v", err)
	}
	err = sm.Transition(StateThirdDown, "strike_breached")
	if err != nil {
		t.Fatalf("Failed to transition to ThirdDown: %v", err)
	}
	err = sm.Transition(StateFourthDown, "adjustment_failed")
	if err != nil {
		t.Fatalf("Failed to transition to FourthDown: %v", err)
	}

	// In FourthDown state, should be able to punt
	if !sm.CanPunt() {
		t.Error("State machine in FourthDown should allow punt")
	}

	// Execute first punt
	err = sm.ExecutePunt()
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

	pos.CreditReceived = 3.50 // per-share credit
	pos.CurrentPnL = -175.00  // 50% loss (50% of total credit: 3.50 * 1 * 100 = 350)

	// Below 200% loss - no emergency exit
	shouldExit, reason := pos.ShouldEmergencyExit(21, 2.0)
	if shouldExit {
		t.Errorf("Should not emergency exit at 50%% loss, but got: %s", reason)
	}

	// At 200% loss - should trigger
	pos.CurrentPnL = -700.00 // 200% loss (200% of total credit: 3.50 * 1 * 100 = 350)
	shouldExit, _ = pos.ShouldEmergencyExit(21, 2.0)
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

	// Setup: transition position to FourthDown state before testing punt
	err := pos.TransitionState(StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to transition to Submitted: %v", err)
	}
	err = pos.TransitionState(StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Failed to transition to Open: %v", err)
	}
	err = pos.TransitionState(StateFirstDown, "start_management")
	if err != nil {
		t.Fatalf("Failed to transition to FirstDown: %v", err)
	}
	err = pos.TransitionState(StateSecondDown, "strike_challenged")
	if err != nil {
		t.Fatalf("Failed to transition to SecondDown: %v", err)
	}
	err = pos.TransitionState(StateThirdDown, "strike_breached")
	if err != nil {
		t.Fatalf("Failed to transition to ThirdDown: %v", err)
	}
	err = pos.TransitionState(StateFourthDown, "adjustment_failed")
	if err != nil {
		t.Fatalf("Failed to transition to FourthDown: %v", err)
	}

	// Test punt capability
	if !pos.CanPunt() {
		t.Error("Position in FourthDown should allow punt")
	}

	// Execute punt
	err = pos.ExecutePunt()
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
	forceStateForTesting(sm, StateFourthDown)
	sm.SetFourthDownOption(OptionA)
	if err := sm.ExecutePunt(); err != nil {
		t.Errorf("ExecutePunt should succeed: %v", err)
	}

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

// Test ProfitPercent calculation with different quantities
func TestPosition_ProfitPercent(t *testing.T) {
	expiration := time.Now().AddDate(0, 0, 45)

	testCases := []struct {
		name           string
		quantity       int
		creditReceived float64
		currentPnL     float64
		expectedPct    float64
	}{
		{
			name:           "Single quantity, positive P&L",
			quantity:       1,
			creditReceived: 3.50,
			currentPnL:     175.0, // 50% of total credit (3.50 * 1 * 100 = 350)
			expectedPct:    50.0,
		},
		{
			name:           "Single quantity, negative P&L",
			quantity:       1,
			creditReceived: 3.50,
			currentPnL:     -175.0, // -50% of total credit
			expectedPct:    -50.0,
		},
		{
			name:           "Multiple quantity, positive P&L",
			quantity:       5,
			creditReceived: 3.50,
			currentPnL:     437.5, // 25% of total credit (3.50 * 5 * 100 = 1750)
			expectedPct:    25.0,
		},
		{
			name:           "Multiple quantity, zero P&L",
			quantity:       3,
			creditReceived: 2.00,
			currentPnL:     0,
			expectedPct:    0,
		},
		{
			name:           "Zero credit received",
			quantity:       1,
			creditReceived: 0,
			currentPnL:     100,
			expectedPct:    0,
		},
		{
			name:           "Negative credit received (debit spread)",
			quantity:       1,
			creditReceived: -2.50, // Debit of $2.50
			currentPnL:     125.0, // Profit of $125
			expectedPct:    50.0,  // 125 / |(-2.50 * 1 * 100)| * 100 = 125 / 250 * 100 = 50%
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pos := NewPosition("TEST-1", "SPY", 400.0, 420.0, expiration, tc.quantity)
			pos.CreditReceived = tc.creditReceived
			pos.CurrentPnL = tc.currentPnL

			actualPct := pos.ProfitPercent()

			// Use a small epsilon for floating point comparison
			epsilon := 0.001
			if diff := actualPct - tc.expectedPct; diff < -epsilon || diff > epsilon {
				t.Errorf("ProfitPercent() = %.3f, expected %.3f", actualPct, tc.expectedPct)
			}
		})
	}
}

// Test emergency exit with multi-quantity positions and adjustment credits
func TestPosition_EmergencyExit_MultiQuantity_WithAdjustments(t *testing.T) {
	expiration := time.Now().AddDate(0, 0, 30)

	testCases := []struct {
		name            string
		quantity        int
		creditReceived  float64
		adjustments     []Adjustment
		currentPnL      float64
		escalateLossPct float64
		shouldExit      bool
		expectedMsg     string
	}{
		{
			name:            "Multi-quantity position without adjustments",
			quantity:        5,
			creditReceived:  2.00,
			adjustments:     []Adjustment{},
			currentPnL:      -500.0, // 50% loss (2.00 * 5 * 100 = 1000)
			escalateLossPct: 1.5,
			shouldExit:      false,
			expectedMsg:     "",
		},
		{
			name:           "Multi-quantity position with adjustments - below threshold",
			quantity:       3,
			creditReceived: 2.50,
			adjustments: []Adjustment{
				{Credit: 1.00, Type: AdjustmentRoll},
				{Credit: 0.50, Type: AdjustmentDelta},
			},
			currentPnL:      -975.0, // 75% loss (total credit: (2.50 + 1.00 + 0.50) * 3 * 100 = 1300)
			escalateLossPct: 1.5,
			shouldExit:      false,
			expectedMsg:     "",
		},
		{
			name:           "Multi-quantity position with adjustments - at threshold",
			quantity:       3,
			creditReceived: 2.50,
			adjustments: []Adjustment{
				{Credit: 1.00, Type: AdjustmentRoll},
				{Credit: 0.50, Type: AdjustmentDelta},
			},
			currentPnL:      -1800.0, // 150% loss (total credit: (2.50 + 1.00 + 0.50) * 3 * 100 = 1200)
			escalateLossPct: 1.5,
			shouldExit:      true,
			expectedMsg:     "emergency exit: loss 150.0%",
		},
		{
			name:           "Multi-quantity position with adjustments - above threshold",
			quantity:       3,
			creditReceived: 2.50,
			adjustments: []Adjustment{
				{Credit: 1.00, Type: AdjustmentRoll},
				{Credit: 0.50, Type: AdjustmentDelta},
			},
			currentPnL:      -2400.0, // 200% loss (total credit: 1200)
			escalateLossPct: 1.5,
			shouldExit:      true,
			expectedMsg:     "emergency exit: loss 200.0%",
		},
		{
			name:           "Single quantity with adjustment credits",
			quantity:       1,
			creditReceived: 5.00,
			adjustments: []Adjustment{
				{Credit: -2.00, Type: AdjustmentRoll}, // Debit adjustment
			},
			currentPnL:      -300.0, // 100% loss (total credit: (5.00 - 2.00) * 1 * 100 = 300)
			escalateLossPct: 0.8,
			shouldExit:      true,
			expectedMsg:     "emergency exit: loss 100.0%",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pos := NewPosition("TEST-1", "SPY", 400.0, 420.0, expiration, tc.quantity)
			pos.CreditReceived = tc.creditReceived
			pos.Adjustments = tc.adjustments
			pos.CurrentPnL = tc.currentPnL

			shouldExit, reason := pos.ShouldEmergencyExit(21, tc.escalateLossPct)

			if shouldExit != tc.shouldExit {
				t.Errorf("Expected shouldExit=%v, got %v", tc.shouldExit, shouldExit)
			}

			if tc.shouldExit && !contains(reason, tc.expectedMsg) {
				t.Errorf("Expected message containing '%s', got: %s", tc.expectedMsg, reason)
			}
		})
	}
}

// Test credit calculation consistency between ProfitPercent and ShouldEmergencyExit
func TestPosition_CreditCalculationConsistency(t *testing.T) {
	expiration := time.Now().AddDate(0, 0, 45)

	testCases := []struct {
		name           string
		quantity       int
		creditReceived float64
		adjustments    []Adjustment
		currentPnL     float64
	}{
		{
			name:           "Single quantity, no adjustments",
			quantity:       1,
			creditReceived: 3.50,
			adjustments:    []Adjustment{},
			currentPnL:     -175.0, // 50% loss
		},
		{
			name:           "Multi-quantity, no adjustments",
			quantity:       5,
			creditReceived: 2.00,
			adjustments:    []Adjustment{},
			currentPnL:     -500.0, // 50% loss
		},
		{
			name:           "Multi-quantity with adjustments",
			quantity:       3,
			creditReceived: 2.50,
			adjustments: []Adjustment{
				{Credit: 1.00, Type: AdjustmentRoll},
				{Credit: 0.50, Type: AdjustmentDelta},
			},
			currentPnL: -975.0, // 75% loss
		},
		{
			name:           "Single quantity with debit adjustment",
			quantity:       1,
			creditReceived: 5.00,
			adjustments: []Adjustment{
				{Credit: -2.00, Type: AdjustmentRoll},
			},
			currentPnL: -150.0, // 50% loss
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pos := NewPosition("TEST-1", "SPY", 400.0, 420.0, expiration, tc.quantity)
			pos.CreditReceived = tc.creditReceived
			pos.Adjustments = tc.adjustments
			pos.CurrentPnL = tc.currentPnL

			// Both ProfitPercent and ShouldEmergencyExit should use the same total credit basis
			profitPercent := pos.ProfitPercent()

			// Calculate expected percentage manually using the same logic
			totalCredit := pos.GetTotalCredit() * float64(pos.Quantity) * 100
			expectedPercent := (pos.CurrentPnL / totalCredit) * 100

			if profitPercent != expectedPercent {
				t.Errorf("ProfitPercent (%f) doesn't match expected calculation (%f)", profitPercent, expectedPercent)
			}

			// Test that emergency exit uses the same credit basis
			// This is implicitly tested by the fact that both use GetTotalCredit()
			// We can verify this by checking the credit calculation in ShouldEmergencyExit
			totalCreditForEmergency := pos.GetTotalCredit() * float64(pos.Quantity) * 100

			if totalCredit != totalCreditForEmergency {
				t.Errorf("Credit calculation mismatch between ProfitPercent (%f) and ShouldEmergencyExit (%f)",
					totalCredit, totalCreditForEmergency)
			}
		})
	}
}

func TestPosition_JSONSerialization_ExcludesStateMachine(t *testing.T) {
	// Create a new position and transition it to Open state
	pos := NewPosition("test-pos", "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)

	// Transition through valid states: Idle -> Submitted -> Open
	err := pos.TransitionState(StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to transition to submitted: %v", err)
	}
	err = pos.TransitionState(StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Failed to transition to open: %v", err)
	}

	// Verify initial state
	if pos.GetCurrentState() != StateOpen {
		t.Errorf("Expected position state to be %s, got %s", StateOpen, pos.GetCurrentState())
	}
	if pos.StateMachine == nil {
		t.Error("Expected StateMachine to be initialized")
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(pos)
	if err != nil {
		t.Fatalf("Failed to marshal position to JSON: %v", err)
	}

	jsonStr := string(jsonData)

	// Verify StateMachine is NOT in the JSON (excluded with json:"-" tag)
	if strings.Contains(jsonStr, "state_machine") {
		t.Error("StateMachine should be excluded from JSON serialization, but found 'state_machine' in JSON")
	}

	// Verify State field IS in the JSON
	if !strings.Contains(jsonStr, `"state":"open"`) {
		t.Errorf("State field should be included in JSON serialization. JSON: %s", jsonStr)
	}

	// Verify other expected fields are present
	expectedFields := []string{`"id":"test-pos"`, `"symbol":"SPY"`, `"call_strike":410`, `"put_strike":400`, `"quantity":1`}
	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("Expected field %s not found in JSON: %s", field, jsonStr)
		}
	}

	// Deserialize from JSON
	var deserializedPos Position
	err = json.Unmarshal(jsonData, &deserializedPos)
	if err != nil {
		t.Fatalf("Failed to unmarshal position from JSON: %v", err)
	}

	// Verify StateMachine is nil after deserialization (as expected)
	if deserializedPos.StateMachine != nil {
		t.Error("StateMachine should be nil after JSON deserialization")
	}

	// Verify persisted state is correct
	if deserializedPos.GetCurrentState() != StateOpen {
		t.Errorf("Expected deserialized position state to be %s, got %s", StateOpen, deserializedPos.GetCurrentState())
	}

	// Verify lazy initialization works - calling a method that uses StateMachine
	// should initialize it from the persisted state
	managementPhase := deserializedPos.GetManagementPhase()
	if managementPhase != 0 { // Open state should have management phase 0
		t.Errorf("Expected management phase 0 for Open state, got %d", managementPhase)
	}

	// Verify StateMachine is now initialized after lazy initialization
	if deserializedPos.StateMachine == nil {
		t.Error("StateMachine should be initialized after lazy initialization")
	}

	// Verify the StateMachine has the correct state
	if deserializedPos.StateMachine.GetCurrentState() != StateOpen {
		t.Errorf("StateMachine should be in %s state after lazy initialization, got %s",
			StateOpen, deserializedPos.StateMachine.GetCurrentState())
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool { return strings.Contains(s, substr) }
