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
	// OptionNone indicates no Fourth Down option has been selected
	OptionNone FourthDownOption = ""
	// OptionA represents aggressive adjustment strategy
	OptionA FourthDownOption = "option_a"
	// OptionB represents conservative adjustment strategy
	OptionB FourthDownOption = "option_b"
	// OptionC represents DTE-based limit strategy
	OptionC FourthDownOption = "option_c"
)

const (
	// StateIdle indicates no active position
	StateIdle PositionState = "idle"
	// StateSubmitted indicates order submitted, waiting for fill
	StateSubmitted PositionState = "submitted"
	// StateOpen indicates position opened, ready for management
	StateOpen PositionState = "open"
	// StateClosed indicates position closed
	StateClosed PositionState = "closed"
	// StateError indicates error state requiring intervention
	StateError PositionState = "error"

	// StateFirstDown indicates normal theta decay monitoring
	StateFirstDown PositionState = "first_down"
	// StateSecondDown represents when strike is challenged (within 5 points)
	StateSecondDown PositionState = "second_down"
	// StateThirdDown represents when strike is breached, consider adjustments
	StateThirdDown PositionState = "third_down"
	// StateFourthDown represents critical decision point
	StateFourthDown PositionState = "fourth_down"

	// StateAdjusting indicates executing position adjustment
	StateAdjusting PositionState = "adjusting"
	// StateRolling indicates rolling to new expiration
	StateRolling PositionState = "rolling"
)

// StateTransition defines valid state transitions
type StateTransition struct {
	From        PositionState
	To          PositionState
	Condition   string
	Description string
}

// ValidTransitions defines the allowed state transitions for the position state machine.
var ValidTransitions = []StateTransition{
	// Position lifecycle
	{StateIdle, StateSubmitted, "order_placed", "Order submitted to broker"},
	{StateIdle, StateFirstDown, "skip_order_entry", "Skipping order entry, going directly to management"},
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
	{StateAdjusting, StateClosed, "hard_stop", "Hard stop triggered during adjustment"},
	{StateRolling, StateClosed, "force_close", "Force close during time roll"},

	// Adjustment transitions
	{StateSecondDown, StateAdjusting, "roll_untested", "Rolling untested side"},
	{StateThirdDown, StateAdjusting, "execute_adjustment", "Executing adjustment strategy"},
	{StateFourthDown, StateRolling, "roll_as_punt", "Rolling to new expiration as punt strategy"},

	// Return from adjustments
	{StateAdjusting, StateFirstDown, "adjustment_complete", "Adjustment completed successfully"},
	{StateRolling, StateFirstDown, "roll_complete", "Time roll completed"},
	{StateAdjusting, StateError, "adjustment_failed", "Adjustment failed"},
	{StateRolling, StateError, "roll_failed", "Time roll failed"},

	// Error recovery
	{StateError, StateIdle, "manual_intervention", "Manual intervention completed"},
	{StateError, StateClosed, "force_close", "Force close position"},
}

// transitionLookup provides O(1) lookup for valid transitions: map[fromState][toState][condition]bool
var transitionLookup map[PositionState]map[PositionState]map[string]bool

// init precomputes the transition lookup map for O(1) lookups
func init() {
	transitionLookup = make(map[PositionState]map[PositionState]map[string]bool)

	for _, transition := range ValidTransitions {
		if transitionLookup[transition.From] == nil {
			transitionLookup[transition.From] = make(map[PositionState]map[string]bool)
		}
		if transitionLookup[transition.From][transition.To] == nil {
			transitionLookup[transition.From][transition.To] = make(map[string]bool)
		}
		transitionLookup[transition.From][transition.To][transition.Condition] = true
	}
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

// NewStateMachine creates a new state machine with default limits
func NewStateMachine() *StateMachine {
	return NewStateMachineWithLimits(3, 1)
}

// NewStateMachineFromState creates a new state machine initialized to a specific state
func NewStateMachineFromState(state PositionState) *StateMachine {
	sm := NewStateMachine()
	sm.currentState = state
	sm.previousState = state // Set previous to same to avoid inconsistency
	sm.transitionTime = time.Now().UTC()
	sm.transitionCount = make(map[PositionState]int)
	// Initialize transition count for the current state to 1 to indicate it's been set
	sm.transitionCount[state] = 1
	return sm
}

// NewStateMachineWithLimits creates a new state machine with configurable limits
func NewStateMachineWithLimits(maxAdj, maxRolls int) *StateMachine {
	return &StateMachine{
		currentState:    StateIdle,
		previousState:   StateIdle,
		transitionTime:  time.Now().UTC(),
		transitionCount: make(map[PositionState]int),
		maxAdjustments:  maxAdj,
		maxTimeRolls:    maxRolls,
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

// isTransitionDefined checks if the transition is defined using O(1) map lookup
func (sm *StateMachine) isTransitionDefined(to PositionState, condition string) bool {
	if fromMap, exists := transitionLookup[sm.currentState]; exists {
		if toMap, exists := fromMap[to]; exists {
			// Check if condition exists in the map
			_, ok := toMap[condition]
			return ok
		}
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

	// Capture current time once for consistency
	now := time.Now().UTC()

	// Perform transition
	sm.previousState = sm.currentState
	sm.currentState = to
	sm.transitionTime = now
	sm.transitionCount[to]++

	// Set Fourth Down start time
	if to == StateFourthDown {
		sm.fourthDownStartTime = now
	}

	return nil
}

// GetTransitionCount returns how many times we've been in a state
func (sm *StateMachine) GetTransitionCount(state PositionState) int {
	return sm.transitionCount[state]
}

// Reset resets the state machine for a new state machine.
// Note: This clears runtime state (transitions, times, counts) but preserves
// configuration limits (maxAdjustments, maxTimeRolls) for reuse.
func (sm *StateMachine) Reset() {
	sm.currentState = StateIdle
	sm.previousState = StateIdle
	sm.transitionTime = time.Now().UTC()
	sm.transitionCount = make(map[PositionState]int)
	sm.fourthDownOption = OptionNone // No option selected
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

// SetMaxAdjustments sets the maximum number of adjustments allowed
func (sm *StateMachine) SetMaxAdjustments(max int) {
	sm.maxAdjustments = max
}

// SetMaxTimeRolls sets the maximum number of time rolls allowed
func (sm *StateMachine) SetMaxTimeRolls(max int) {
	sm.maxTimeRolls = max
}

// GetMaxAdjustments returns the maximum number of adjustments allowed
func (sm *StateMachine) GetMaxAdjustments() int {
	return sm.maxAdjustments
}

// GetMaxTimeRolls returns the maximum number of time rolls allowed
func (sm *StateMachine) GetMaxTimeRolls() int {
	return sm.maxTimeRolls
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
		return "Rolling: Extending position to new expiration (may be part of punt strategy)"
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

// ShouldEmergencyExit checks emergency exit conditions.
// creditBasis MUST use the same unit basis as currentPnL (e.g., total $ P&L incl. qty×100).
// escalateLossPct is a decimal (e.g., 2.5 = 250%).
func (sm *StateMachine) ShouldEmergencyExit(
	creditBasis, currentPnL, dte float64, maxDTE int, escalateLossPct float64) (bool, string) {
	if creditBasis == 0 {
		return false, ""
	}
	lossPercent := (currentPnL / creditBasis) * -100 // Negative because P&L is negative for losses

	// Emergency exit at configured escalate loss percentage (always applies)
	if lossPercent >= escalateLossPct*100 {
		return true, fmt.Sprintf("emergency exit: loss %.1f%% >= %.0f%% threshold", lossPercent, escalateLossPct*100)
	}

	// Check Fourth Down time-based limits if in Fourth Down state
	if sm.currentState == StateFourthDown && !sm.fourthDownStartTime.IsZero() {
		nowDay := time.Now().UTC().Truncate(24 * time.Hour)
		startDay := sm.fourthDownStartTime.UTC().Truncate(24 * time.Hour)
		elapsedDays := int(nowDay.Sub(startDay) / (24 * time.Hour))

		switch sm.fourthDownOption {
		case OptionA:
			if elapsedDays >= 5 {
				return true, "emergency exit: Option A exceeded 5-day limit"
			}
		case OptionB:
			if elapsedDays >= 3 {
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

// ExecutePunt performs punt operation (FourthDown rescue mechanism)
func (sm *StateMachine) ExecutePunt() error {
	if sm.currentState != StateFourthDown {
		return fmt.Errorf("punt only allowed from FourthDown state, current state: %s", sm.currentState)
	}

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

// SkipOrderEntry allows skipping the order entry process and going directly to management
func (sm *StateMachine) SkipOrderEntry() error {
	if sm.currentState != StateIdle {
		return fmt.Errorf("skip order entry only allowed from Idle state, current state: %s", sm.currentState)
	}

	return sm.Transition(StateFirstDown, "skip_order_entry")
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
