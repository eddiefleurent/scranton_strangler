// Package models provides data structures and state management for trading positions.
package models

import (
	"fmt"
	"time"
)

// PositionState represents the current state of a position
type PositionState string

// FourthDownOption represents the strategy choice for Fourth Down situations
type FourthDownOption string

const (
	OptionA FourthDownOption = "option_a" // Aggressive adjustment
	OptionB FourthDownOption = "option_b" // Conservative adjustment
	OptionC FourthDownOption = "option_c" // DTE-based limit
)

const (
	// Core states
	StateIdle      PositionState = "idle"      // No active position
	StateSubmitted PositionState = "submitted" // Order submitted, waiting for fill
	StateOpen      PositionState = "open"      // Position opened, ready for management
	StateClosed    PositionState = "closed"    // Position closed
	StateError     PositionState = "error"     // Error state requiring intervention

	// Football System management states
	StateFirstDown  PositionState = "first_down"  // Normal theta decay monitoring
	StateSecondDown PositionState = "second_down" // Strike challenged (within 5 points)
	StateThirdDown  PositionState = "third_down"  // Strike breached, consider adjustments
	StateFourthDown PositionState = "fourth_down" // Critical decision point

	// Action states
	StateAdjusting PositionState = "adjusting" // Executing position adjustment
	StateRolling   PositionState = "rolling"   // Rolling to new expiration
)

// StateTransition defines valid state transitions
type StateTransition struct {
	From        PositionState
	To          PositionState
	Condition   string
	Description string
}

// Valid state transitions (simplified)
var ValidTransitions = []StateTransition{
	// Position lifecycle
	{StateIdle, StateSubmitted, "order_placed", "Order submitted to broker"},
	{StateIdle, StateFirstDown, "punt_executed", "Punting on position entry"},
	{StateSubmitted, StateOpen, "order_filled", "Order filled successfully"},
	{StateSubmitted, StateError, "order_failed", "Order failed or canceled"},
	{StateSubmitted, StateClosed, "order_timeout", "Order timed out without fill"},
	{StateOpen, StateFirstDown, "start_management", "Begin football system monitoring"},
	{StateOpen, StateClosed, "position_closed", "Position closed directly (profit target hit, time limit, etc.)"},

	// Football System progression
	{StateFirstDown, StateSecondDown, "strike_challenged", "Price within 5 points of strike"},
	{StateSecondDown, StateThirdDown, "strike_breached", "Price breached strike"},
	{StateThirdDown, StateFourthDown, "adjustment_failed", "Adjustment attempt failed"},

	// Recovery transitions
	{StateSecondDown, StateFirstDown, "price_recovered", "Price moved away from strike"},
	{StateThirdDown, StateFirstDown, "adjustment_successful", "Successfully adjusted position"},
	{StateFourthDown, StateFirstDown, "recovery_successful", "Position recovered after critical adjustment"},
	{StateFourthDown, StateFirstDown, "punt_executed", "Punting position in fourth down"},

	// Exit transitions from any management state
	{StateFirstDown, StateClosed, "exit_conditions", "Profit target or time limit reached"},
	{StateSecondDown, StateClosed, "exit_conditions", "Exit conditions met"},
	{StateThirdDown, StateClosed, "hard_stop", "Hard stop triggered"},
	{StateFourthDown, StateClosed, "emergency_exit", "Emergency exit required"},

	// Adjustment transitions
	{StateSecondDown, StateAdjusting, "roll_untested", "Rolling untested side"},
	{StateThirdDown, StateAdjusting, "execute_adjustment", "Executing adjustment strategy"},
	{StateFourthDown, StateRolling, "punt_decision", "Rolling to new expiration (punt)"},

	// Return from adjustments
	{StateAdjusting, StateFirstDown, "adjustment_complete", "Adjustment completed successfully"},
	{StateRolling, StateFirstDown, "roll_complete", "Time roll completed"},
	{StateAdjusting, StateError, "adjustment_failed", "Adjustment failed"},
	{StateRolling, StateError, "roll_failed", "Time roll failed"},

	// Error recovery
	{StateError, StateIdle, "manual_intervention", "Manual intervention completed"},
	{StateError, StateClosed, "force_close", "Force close position"},
}

// StateMachine manages position state transitions
type StateMachine struct {
	transitionTime      time.Time
	fourthDownStartTime time.Time
	transitionCount     map[PositionState]int
	currentState        PositionState
	previousState       PositionState
	fourthDownOption    FourthDownOption
	maxAdjustments      int
	maxTimeRolls        int
	puntCount           int
}

// NewStateMachine creates a new state machine
func NewStateMachine() *StateMachine {
	return &StateMachine{
		currentState:    StateIdle,
		previousState:   StateIdle,
		transitionTime:  time.Now().UTC(),
		transitionCount: make(map[PositionState]int),
		maxAdjustments:  3, // Max 3 strike adjustments per trade
		maxTimeRolls:    1, // Max 1 time roll per trade
	}
}

// GetCurrentState returns the current state
func (sm *StateMachine) GetCurrentState() PositionState {
	return sm.currentState
}

// GetPreviousState returns the previous state
func (sm *StateMachine) GetPreviousState() PositionState {
	return sm.previousState
}

// IsValidTransition checks if a transition is valid
func (sm *StateMachine) IsValidTransition(to PositionState, condition string) error {
	if !sm.isTransitionDefined(to, condition) {
		return fmt.Errorf("invalid transition from %s to %s with condition '%s'",
			sm.currentState, to, condition)
	}

	return sm.validateTransitionLimits(to)
}

// isTransitionDefined checks if the transition is defined in ValidTransitions
func (sm *StateMachine) isTransitionDefined(to PositionState, condition string) bool {
	for _, transition := range ValidTransitions {
		if sm.matchesTransition(transition, to, condition) {
			return true
		}
	}
	return false
}

// matchesTransition checks if a transition matches the given state and condition
func (sm *StateMachine) matchesTransition(transition StateTransition, to PositionState, condition string) bool {
	if transition.From != sm.currentState || transition.To != to {
		return false
	}
	return sm.conditionMatches(transition.Condition, condition)
}

// conditionMatches checks if the condition requirements are satisfied
func (sm *StateMachine) conditionMatches(transitionCondition, providedCondition string) bool {
	// No condition required and none provided
	if transitionCondition == "" && providedCondition == "" {
		return true
	}
	// Condition required and matches
	if transitionCondition != "" && providedCondition != "" && providedCondition == transitionCondition {
		return true
	}
	// No condition required but one provided (allowed)
	if transitionCondition == "" && providedCondition != "" {
		return true
	}
	return false
}

// validateTransitionLimits checks if transition limits are respected
func (sm *StateMachine) validateTransitionLimits(to PositionState) error {
	if to == StateAdjusting && sm.transitionCount[StateAdjusting] >= sm.maxAdjustments {
		return fmt.Errorf("maximum adjustments (%d) exceeded", sm.maxAdjustments)
	}
	if to == StateRolling && sm.transitionCount[StateRolling] >= sm.maxTimeRolls {
		return fmt.Errorf("maximum time rolls (%d) exceeded", sm.maxTimeRolls)
	}
	return nil
}

// Transition moves to a new state
func (sm *StateMachine) Transition(to PositionState, condition string) error {
	// Validate transition
	if err := sm.IsValidTransition(to, condition); err != nil {
		return err
	}

	// Perform transition
	sm.previousState = sm.currentState
	sm.currentState = to
	sm.transitionTime = time.Now().UTC()
	sm.transitionCount[to]++

	// Set Fourth Down start time
	if to == StateFourthDown {
		sm.fourthDownStartTime = time.Now().UTC()
	}

	return nil
}

// GetTransitionCount returns how many times we've been in a state
func (sm *StateMachine) GetTransitionCount(state PositionState) int {
	return sm.transitionCount[state]
}

// Reset resets the state machine for a new state machine
func (sm *StateMachine) Reset() {
	sm.currentState = StateIdle
	sm.previousState = StateIdle
	sm.transitionTime = time.Now().UTC()
	sm.transitionCount = make(map[PositionState]int)
	sm.fourthDownOption = ""
	sm.fourthDownStartTime = time.Time{}
	sm.puntCount = 0
}

// IsManagementState returns true if we're in a football management state
func (sm *StateMachine) IsManagementState() bool {
	return sm.currentState == StateFirstDown ||
		sm.currentState == StateSecondDown ||
		sm.currentState == StateThirdDown ||
		sm.currentState == StateFourthDown
}

// GetManagementPhase returns which "down" we're in (1-4)
func (sm *StateMachine) GetManagementPhase() int {
	switch sm.currentState {
	case StateFirstDown:
		return 1
	case StateSecondDown:
		return 2
	case StateThirdDown:
		return 3
	case StateFourthDown:
		return 4
	default:
		return 0
	}
}

// CanAdjust returns true if adjustments are still allowed
func (sm *StateMachine) CanAdjust() bool {
	return sm.transitionCount[StateAdjusting] < sm.maxAdjustments
}

// CanRoll returns true if time rolls are still allowed
func (sm *StateMachine) CanRoll() bool {
	return sm.transitionCount[StateRolling] < sm.maxTimeRolls
}

// GetStateDescription returns a human-readable description of the current state
func (sm *StateMachine) GetStateDescription() string {
	switch sm.currentState {
	case StateIdle:
		return "No active position, ready for new opportunities"
	case StateSubmitted:
		return "Order submitted, waiting for broker confirmation"
	case StateOpen:
		return "Position opened, transitioning to management"
	case StateFirstDown:
		return "First Down: Normal monitoring - position healthy, collecting theta"
	case StateSecondDown:
		return "Second Down: Strike challenged - price within 5 points, elevated monitoring"
	case StateThirdDown:
		return "Third Down: Strike breached - considering adjustments or defensive actions"
	case StateFourthDown:
		return "Fourth Down: Critical situation - final adjustment attempt or prepare to exit"
	case StateAdjusting:
		return "Executing position adjustment"
	case StateRolling:
		return "Punting: Rolling position to new expiration"
	case StateClosed:
		return "Position closed, ready for next opportunity"
	case StateError:
		return "Error state - manual intervention required"
	default:
		return "Unknown state"
	}
}

// ValidateStateConsistency ensures the state machine is in a valid state
func (sm *StateMachine) ValidateStateConsistency() error {
	// For fresh state machines (no transitions recorded at all), allow initial state
	totalTransitions := 0
	for _, count := range sm.transitionCount {
		totalTransitions += count
	}

	if totalTransitions == 0 && sm.currentState == StateIdle && sm.previousState == StateIdle {
		return nil
	}

	// Ensure transitionTime is set for state machines that have performed transitions
	if sm.transitionTime.IsZero() && totalTransitions > 0 {
		return fmt.Errorf("missing transition time: transitionTime is zero")
	}

	// Ensure that when currentState equals previousState there is at least one recorded transition for that state
	if sm.currentState == sm.previousState && sm.transitionCount[sm.currentState] == 0 && totalTransitions > 0 {
		return fmt.Errorf("inconsistent transition counts for identical states: "+
			"current and previous states are the same (%s) but no transitions recorded", sm.currentState)
	}

	// Check adjustment/roll limits
	if sm.transitionCount[StateAdjusting] > sm.maxAdjustments {
		return fmt.Errorf("adjustment count %d exceeds maximum %d",
			sm.transitionCount[StateAdjusting], sm.maxAdjustments)
	}

	if sm.transitionCount[StateRolling] > sm.maxTimeRolls {
		return fmt.Errorf("time roll count %d exceeds maximum %d",
			sm.transitionCount[StateRolling], sm.maxTimeRolls)
	}

	return nil
}

// ShouldEmergencyExit checks if the position meets emergency exit conditions
func (sm *StateMachine) ShouldEmergencyExit(
	creditReceived, currentPnL, dte float64, maxDTE int, escalateLossPct float64) (bool, string) {
	// Calculate loss percentage
	if creditReceived == 0 {
		return false, ""
	}
	lossPercent := (currentPnL / creditReceived) * -100 // Negative because P&L is negative for losses

	// Emergency exit at configured escalate loss percentage (always applies)
	if lossPercent >= escalateLossPct*100 {
		return true, fmt.Sprintf("emergency exit: loss %.1f%% >= %.0f%% threshold", lossPercent, escalateLossPct*100)
	}

	// Check Fourth Down time-based limits if in Fourth Down state
	if sm.currentState == StateFourthDown && !sm.fourthDownStartTime.IsZero() {
		daysInFourthDown := time.Since(sm.fourthDownStartTime).Hours() / 24

		switch sm.fourthDownOption {
		case OptionA:
			if daysInFourthDown >= 6 { // Test expects > 5 days, so >= 6
				return true, "emergency exit: Option A exceeded 5-day limit"
			}
		case OptionB:
			if daysInFourthDown >= 4 { // Test expects > 3 days, so >= 4
				return true, "emergency exit: Option B exceeded 3-day limit"
			}
		case OptionC:
			if dte <= float64(maxDTE) {
				return true, fmt.Sprintf("emergency exit: Option C reached %d DTE limit", maxDTE)
			}
		}
	}

	return false, ""
}

// SetFourthDownOption sets the Fourth Down strategy option
func (sm *StateMachine) SetFourthDownOption(option FourthDownOption) {
	sm.fourthDownOption = option
}

// GetFourthDownOption returns the selected Fourth Down strategy
func (sm *StateMachine) GetFourthDownOption() FourthDownOption {
	return sm.fourthDownOption
}

// CanPunt returns true if punt is still allowed
func (sm *StateMachine) CanPunt() bool {
	return sm.puntCount == 0 // Allow only one punt per position
}

// ExecutePunt performs punt operation
func (sm *StateMachine) ExecutePunt() error {
	if !sm.CanPunt() {
		return fmt.Errorf("punt not allowed: already used")
	}

	err := sm.Transition(StateFirstDown, "punt_executed")
	if err != nil {
		return err
	}
	sm.puntCount++
	return nil
}

// Copy creates a deep copy of the StateMachine
func (sm *StateMachine) Copy() *StateMachine {
	if sm == nil {
		return nil
	}

	newSM := &StateMachine{
		currentState:        sm.currentState,
		previousState:       sm.previousState,
		transitionTime:      sm.transitionTime,
		maxAdjustments:      sm.maxAdjustments,
		maxTimeRolls:        sm.maxTimeRolls,
		fourthDownOption:    sm.fourthDownOption,
		fourthDownStartTime: sm.fourthDownStartTime,
		puntCount:           sm.puntCount,
	}

	// Deep copy transitionCount map
	newSM.transitionCount = make(map[PositionState]int)
	for k, v := range sm.transitionCount {
		newSM.transitionCount[k] = v
	}

	return newSM
}
