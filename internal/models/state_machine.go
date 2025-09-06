package models

import (
	"fmt"
	"time"
)

// PositionState represents the current state of a position
type PositionState string

const (
	// Core states
	StateIdle   PositionState = "idle"   // No active position
	StateOpen   PositionState = "open"   // Position opened, ready for management
	StateClosed PositionState = "closed" // Position closed
	StateError  PositionState = "error"  // Error state requiring intervention

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
	{StateIdle, StateOpen, "position_filled", "Position opened successfully"},
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
	currentState    PositionState
	previousState   PositionState
	transitionTime  time.Time
	transitionCount map[PositionState]int
	maxAdjustments  int // Maximum number of adjustments allowed
	maxTimeRolls    int // Maximum number of time rolls allowed
}

// NewStateMachine creates a new state machine
func NewStateMachine() *StateMachine {
	return &StateMachine{
		currentState:    StateIdle,
		previousState:   StateIdle,
		transitionTime:  time.Now(),
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
	// Check if this transition is defined
	for _, transition := range ValidTransitions {
		if transition.From == sm.currentState && transition.To == to {
			// Additional validation for adjustment limits
			if to == StateAdjusting && sm.transitionCount[StateAdjusting] >= sm.maxAdjustments {
				return fmt.Errorf("maximum adjustments (%d) exceeded", sm.maxAdjustments)
			}
			if to == StateRolling && sm.transitionCount[StateRolling] >= sm.maxTimeRolls {
				return fmt.Errorf("maximum time rolls (%d) exceeded", sm.maxTimeRolls)
			}
			return nil
		}
	}

	return fmt.Errorf("invalid transition from %s to %s with condition '%s'",
		sm.currentState, to, condition)
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
	sm.transitionTime = time.Now()
	sm.transitionCount[to]++

	return nil
}

// GetTransitionCount returns how many times we've been in a state
func (sm *StateMachine) GetTransitionCount(state PositionState) int {
	return sm.transitionCount[state]
}

// Reset resets the state machine for a new position
func (sm *StateMachine) Reset() {
	sm.currentState = StateIdle
	sm.previousState = StateIdle
	sm.transitionTime = time.Now()
	sm.transitionCount = make(map[PositionState]int)
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
	// Check for impossible state combinations
	if sm.currentState == sm.previousState && sm.transitionTime.IsZero() {
		return fmt.Errorf("invalid state: current and previous states are the same with no transition time")
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
