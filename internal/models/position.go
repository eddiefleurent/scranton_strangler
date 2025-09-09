package models

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const sharesPerContract = 100.0

// IVReading represents a single implied volatility reading for a symbol on a specific date
type IVReading struct {
	Symbol    string    `json:"symbol"`
	Date      time.Time `json:"date"`
	IV        float64   `json:"iv"`        // Implied volatility as decimal (0.20 = 20%)
	Timestamp time.Time `json:"timestamp"` // When this reading was recorded
}

// Position represents a short strangle trading position with state management.
type Position struct {
	StateMachine   *StateMachine `json:"-"`     // Runtime only, excluded from JSON
	State          PositionState `json:"state"` // Canonical persisted state
	Adjustments    []Adjustment  `json:"adjustments"`
	ID             string        `json:"id"`
	Symbol         string        `json:"symbol"`
	EntryOrderID   string        `json:"entry_order_id,omitempty"`
	ExitOrderID    string        `json:"exit_order_id,omitempty"`
	ExitReason     string        `json:"exit_reason,omitempty"`
	Expiration     time.Time     `json:"expiration"`
	EntryDate      time.Time     `json:"entry_date,omitempty"`
	ExitDate       time.Time     `json:"exit_date,omitempty"`
	CreditReceived   float64       `json:"credit_received"`
	EntryLimitPrice float64       `json:"entry_limit_price"`
	EntryIV         float64       `json:"entry_iv"`
	EntrySpot       float64       `json:"entry_spot"`
	CurrentPnL     float64       `json:"current_pnl"`
	CallStrike     float64       `json:"call_strike"`
	PutStrike      float64       `json:"put_strike"`
	Quantity       int           `json:"quantity"`
	// DTE is derived; avoid persisting to prevent staleness
	DTE int `json:"-"`
}

// AdjustmentType defines the type of adjustment made to a position.
type AdjustmentType string

const (
	// AdjustmentRoll indicates a rolling adjustment
	AdjustmentRoll AdjustmentType = "roll"
	// AdjustmentDelta indicates a delta adjustment
	AdjustmentDelta AdjustmentType = "delta"
	// AdjustmentHedge indicates a hedging adjustment
	AdjustmentHedge AdjustmentType = "hedge"
)

// Valid returns true if the AdjustmentType is one of the defined constants
func (t AdjustmentType) Valid() bool {
	switch t {
	case AdjustmentRoll, AdjustmentDelta, AdjustmentHedge:
		return true
	default:
		return false
	}
}

// Adjustment represents a modification made to an existing position.
type Adjustment struct {
	Date        time.Time      `json:"date"`
	Type        AdjustmentType `json:"type"`
	Description string         `json:"description"`
	OldStrike   float64        `json:"old_strike"`
	NewStrike   float64        `json:"new_strike"`
	Credit      float64        `json:"credit"`
}

// CalculateDTE calculates and returns the days to expiration for the position.
func (p *Position) CalculateDTE() int {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	exp := p.Expiration.UTC().Truncate(24 * time.Hour)
	days := int(exp.Sub(now).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// GetNetCredit returns the net credit received including adjustments (can be negative if adjustments include debits).
func (p *Position) GetNetCredit() float64 {
	total := p.CreditReceived
	for _, adj := range p.Adjustments {
		total += adj.Credit
	}
	return total
}

// GetTotalCredit returns the total credit received including adjustments.
// Deprecated: Use GetNetCredit instead for better semantic clarity.
func (p *Position) GetTotalCredit() float64 {
	return p.GetNetCredit()
}

// ProfitPercent returns P/L as a percentage of initial credit.
// May be negative (loss) and can exceed 100% with adjustments.
func (p *Position) ProfitPercent() float64 {
	denom := math.Abs(p.GetNetCredit() * float64(p.Quantity) * sharesPerContract)
	if denom == 0 {
		return 0
	}
	return (p.CurrentPnL / denom) * 100
}

// NewPosition creates a new position with initialized state machine
func NewPosition(id, symbol string, putStrike, callStrike float64, expiration time.Time, quantity int) *Position {
	return &Position{
		ID:           id,
		Symbol:       symbol,
		PutStrike:    putStrike,
		CallStrike:   callStrike,
		Expiration:   expiration,
		Quantity:     quantity,
		Adjustments:  make([]Adjustment, 0),
		StateMachine: NewStateMachine(),
		State:        StateIdle,   // Initialize with canonical persisted state
		ExitDate:     time.Time{}, // Initialize as zero time
	}
}

// TransitionState moves the position to a new state
func (p *Position) TransitionState(to PositionState, condition string) error {
	err := p.ensureMachine().Transition(to, condition)
	if err != nil {
		return fmt.Errorf("position %s state transition failed: %w", p.ID, err)
	}

	// Update canonical state
	p.State = to

	// Set EntryDate when transitioning to open state (only if not already set)
	if to == StateOpen && p.EntryDate.IsZero() {
		p.EntryDate = time.Now().UTC()
	}

	// Set ExitDate when transitioning to closed state (only if not already set)
	if to == StateClosed && p.ExitDate.IsZero() {
		p.ExitDate = time.Now().UTC()
	}

	// Clear fields when transitioning to submitted state (pre-entry state)
	if to == StateSubmitted {
		p.clearNonActiveFields()
		p.EntryLimitPrice = 0 // Additional reset specific to submitted state
	}

	// Clear fields when transitioning to error state (like idle/invalid)
	if to == StateError {
		p.clearNonActiveFields()
		p.EntryLimitPrice = 0 // Additional reset specific to error state
	}

	return nil
}

// clearNonActiveFields resets fields that should be cleared when transitioning to non-active states
func (p *Position) clearNonActiveFields() {
	p.EntryDate = time.Time{}
	p.ExitDate = time.Time{}
	p.ExitReason = ""
	p.CreditReceived = 0
	p.Quantity = 0
	p.Adjustments = make([]Adjustment, 0)
}

// GetCurrentState returns the canonical persisted state
func (p *Position) GetCurrentState() PositionState {
	return p.State
}

// ensureMachine ensures the StateMachine is initialized from persisted state
func (p *Position) ensureMachine() *StateMachine {
	if p.StateMachine == nil {
		p.StateMachine = NewStateMachineFromState(p.State)
	}
	return p.StateMachine
}

// IsInManagement returns true if position is in football management states
func (p *Position) IsInManagement() bool {
	return p.ensureMachine().IsManagementState()
}

// GetManagementPhase returns which "down" we're in (1-4)
func (p *Position) GetManagementPhase() int {
	return p.ensureMachine().GetManagementPhase()
}

// CanAdjust returns true if more adjustments are allowed
func (p *Position) CanAdjust() bool {
	return p.ensureMachine().CanAdjust()
}

// CanRoll returns true if time rolls are still allowed
func (p *Position) CanRoll() bool {
	return p.ensureMachine().CanRoll()
}

// ValidateState ensures the position state is consistent with strong invariants
func (p *Position) ValidateState() error {
	// Validate state machine consistency
	if err := p.ensureMachine().ValidateStateConsistency(); err != nil {
		return fmt.Errorf("position %s state validation failed: %w", p.ID, err)
	}

	// Validate position data consistency with state
	currentState := p.State

	// Global invariant: CreditReceived must never be negative
	if p.CreditReceived < 0 {
		return fmt.Errorf("position %s in state %s: CreditReceived cannot be negative (current: %.2f)",
			p.ID, currentState, p.CreditReceived)
	}

	// Check if position data is consistent with state
	switch currentState {
	case StateIdle:
		// Idle state invariants: no active position data
		if !p.EntryDate.IsZero() {
			return fmt.Errorf("position %s in state %s: EntryDate must be zero for idle positions (current: %v)",
				p.ID, currentState, p.EntryDate)
		}
		if !p.ExitDate.IsZero() {
			return fmt.Errorf("position %s in state %s: ExitDate must be zero for non-closed positions (current: %v)",
				p.ID, currentState, p.ExitDate)
		}
		if strings.TrimSpace(p.ExitReason) != "" {
			return fmt.Errorf("position %s in state %s: ExitReason must be empty for non-closed positions (current: %s)",
				p.ID, currentState, p.ExitReason)
		}
		if p.CreditReceived != 0 {
			return fmt.Errorf("position %s in state %s: CreditReceived must be zero for idle positions (current: %.2f)",
				p.ID, currentState, p.CreditReceived)
		}
		if len(p.Adjustments) > 0 {
			return fmt.Errorf("position %s in state %s: Adjustments must be empty for idle positions (current: %d)",
				p.ID, currentState, len(p.Adjustments))
		}
	case StateOpen, StateFirstDown, StateSecondDown, StateThirdDown, StateFourthDown:
		// Active position invariants: must have entry data and positive credit
		if p.EntryDate.IsZero() {
			return fmt.Errorf("position %s in state %s: EntryDate must be set for active positions",
				p.ID, currentState)
		}
		if !p.ExitDate.IsZero() {
			return fmt.Errorf("position %s in state %s: ExitDate must be zero for non-closed positions (current: %v)",
				p.ID, currentState, p.ExitDate)
		}
		if strings.TrimSpace(p.ExitReason) != "" {
			return fmt.Errorf("position %s in state %s: ExitReason must be empty for non-closed positions (current: %s)",
				p.ID, currentState, p.ExitReason)
		}
		if p.CreditReceived <= 0 {
			return fmt.Errorf("position %s in state %s: CreditReceived must be positive for active positions (current: %.2f)",
				p.ID, currentState, p.CreditReceived)
		}
		if p.Quantity <= 0 {
			return fmt.Errorf("position %s in state %s: Quantity must be > 0 for active positions (current: %d)",
				p.ID, currentState, p.Quantity)
		}
	case StateClosed:
		// Closed position invariants: must have complete lifecycle data
		if p.EntryDate.IsZero() {
			return fmt.Errorf("position %s in state %s: EntryDate must be set for closed positions",
				p.ID, currentState)
		}
		if p.ExitDate.IsZero() {
			return fmt.Errorf("position %s in state %s: ExitDate must be set for closed positions",
				p.ID, currentState)
		}
		if strings.TrimSpace(p.ExitReason) == "" {
			return fmt.Errorf("position %s in state %s: ExitReason must be set for closed positions", p.ID, currentState)
		}
		if p.CreditReceived <= 0 {
			return fmt.Errorf("position %s in state %s: CreditReceived must be positive for closed positions (current: %.2f)",
				p.ID, currentState, p.CreditReceived)
		}
		if p.Quantity <= 0 {
			return fmt.Errorf("position %s in state %s: Quantity must be > 0 for closed positions (current: %d)",
				p.ID, currentState, p.Quantity)
		}
		// Validate temporal ordering: EntryDate must be before ExitDate
		if !p.EntryDate.Before(p.ExitDate) {
			return fmt.Errorf("position %s in state %s: EntryDate (%v) must be before ExitDate (%v)",
				p.ID, currentState, p.EntryDate, p.ExitDate)
		}
	case StateSubmitted:
		// Submitted-state invariants are strict (credit=0, qty=0):
		// StateSubmitted represents "order placed but not yet filled".
		// Credit and quantity should remain zero until order fills and position becomes Open.
		// This maintains clear separation between order placement and position activation.
		if !p.EntryDate.IsZero() {
			return fmt.Errorf("position %s in state %s: EntryDate must be zero for submitted positions (current: %v)",
				p.ID, currentState, p.EntryDate)
		}
		if !p.ExitDate.IsZero() {
			return fmt.Errorf("position %s in state %s: ExitDate must be zero for non-closed positions (current: %v)",
				p.ID, currentState, p.ExitDate)
		}
		if strings.TrimSpace(p.ExitReason) != "" {
			return fmt.Errorf("position %s in state %s: ExitReason must be empty for non-closed positions (current: %s)",
				p.ID, currentState, p.ExitReason)
		}
		if p.CreditReceived != 0 {
			return fmt.Errorf("position %s in state %s: CreditReceived must be zero for submitted positions (current: %.2f)",
				p.ID, currentState, p.CreditReceived)
		}
		if p.Quantity != 0 {
			return fmt.Errorf("position %s in state %s: Quantity must be zero for submitted positions (current: %d)",
				p.ID, currentState, p.Quantity)
		}
		if len(p.Adjustments) > 0 {
			return fmt.Errorf("position %s in state %s: Adjustments must be empty for submitted positions (current: %d)",
				p.ID, currentState, len(p.Adjustments))
		}
	case StateAdjusting:
		// Adjusting state invariants: position active but undergoing adjustment
		if p.EntryDate.IsZero() {
			return fmt.Errorf("position %s in state %s: EntryDate must be set for adjusting positions",
				p.ID, currentState)
		}
		if !p.ExitDate.IsZero() {
			return fmt.Errorf("position %s in state %s: ExitDate must be zero for non-closed positions (current: %v)",
				p.ID, currentState, p.ExitDate)
		}
		if strings.TrimSpace(p.ExitReason) != "" {
			return fmt.Errorf("position %s in state %s: ExitReason must be empty for non-closed positions (current: %s)",
				p.ID, currentState, p.ExitReason)
		}
		if p.CreditReceived < 0 {
			return fmt.Errorf("position %s in state %s: CreditReceived must be >= 0 for adjusting positions (current: %.2f)",
				p.ID, currentState, p.CreditReceived)
		}
		if p.Quantity < 0 {
			return fmt.Errorf("position %s in state %s: Quantity must be >= 0 for adjusting positions (current: %d)",
				p.ID, currentState, p.Quantity)
		}
		if len(p.Adjustments) == 0 {
			return fmt.Errorf("position %s in state %s: Adjustments must be non-empty for adjusting positions",
				p.ID, currentState)
		}
	case StateRolling:
		// Rolling state invariants: treat like active state with entry data and positive credit
		if p.EntryDate.IsZero() {
			return fmt.Errorf("position %s in state %s: EntryDate must be set for rolling positions",
				p.ID, currentState)
		}
		if !p.ExitDate.IsZero() {
			return fmt.Errorf("position %s in state %s: ExitDate must be zero for non-closed positions (current: %v)",
				p.ID, currentState, p.ExitDate)
		}
		if strings.TrimSpace(p.ExitReason) != "" {
			return fmt.Errorf("position %s in state %s: ExitReason must be empty for non-closed positions (current: %s)",
				p.ID, currentState, p.ExitReason)
		}
		if p.CreditReceived <= 0 {
			return fmt.Errorf("position %s in state %s: CreditReceived must be positive for rolling positions (current: %.2f)",
				p.ID, currentState, p.CreditReceived)
		}
		if p.Quantity <= 0 {
			return fmt.Errorf("position %s in state %s: Quantity must be > 0 for rolling positions (current: %d)",
				p.ID, currentState, p.Quantity)
		}
	case StateError:
		// Error state invariants: clear most fields like idle/invalid state
		if !p.EntryDate.IsZero() {
			return fmt.Errorf("position %s in state %s: EntryDate must be zero for error positions (current: %v)",
				p.ID, currentState, p.EntryDate)
		}
		if !p.ExitDate.IsZero() {
			return fmt.Errorf("position %s in state %s: ExitDate must be zero for non-closed positions (current: %v)",
				p.ID, currentState, p.ExitDate)
		}
		if strings.TrimSpace(p.ExitReason) != "" {
			return fmt.Errorf("position %s in state %s: ExitReason must be empty for non-closed positions (current: %s)",
				p.ID, currentState, p.ExitReason)
		}
		if p.CreditReceived != 0 {
			return fmt.Errorf("position %s in state %s: CreditReceived must be zero for error positions (current: %.2f)",
				p.ID, currentState, p.CreditReceived)
		}
		if p.Quantity != 0 {
			return fmt.Errorf("position %s in state %s: Quantity must be zero for error positions (current: %d)",
				p.ID, currentState, p.Quantity)
		}
		if len(p.Adjustments) > 0 {
			return fmt.Errorf("position %s in state %s: Adjustments must be empty for error positions (current: %d)",
				p.ID, currentState, len(p.Adjustments))
		}
	}

	// Additional temporal validation for any position with ExitDate set
	if !p.ExitDate.IsZero() && !p.EntryDate.IsZero() && !p.EntryDate.Before(p.ExitDate) {
		return fmt.Errorf("position %s in state %s: EntryDate (%v) must be before ExitDate (%v) when both are set",
			p.ID, currentState, p.EntryDate, p.ExitDate)
	}

	return nil
}

// GetStateDescription returns a human-readable state description
func (p *Position) GetStateDescription() string {
	return p.ensureMachine().GetStateDescription()
}

// ShouldEmergencyExit checks if the position meets emergency exit conditions
func (p *Position) ShouldEmergencyExit(maxDTE int, escalateLossPct float64) (bool, string) {
	dte := p.CalculateDTE()
	// Convert total credit to total dollars for consistent units (includes adjustments)
	totalCredit := p.GetNetCredit() * float64(p.Quantity) * sharesPerContract
	return p.ensureMachine().ShouldEmergencyExit(
		totalCredit, p.CurrentPnL, float64(dte), maxDTE, escalateLossPct)
}

// SetFourthDownOption sets the Fourth Down strategy option
func (p *Position) SetFourthDownOption(option FourthDownOption) {
	p.ensureMachine().SetFourthDownOption(option)
}

// GetFourthDownOption returns the selected Fourth Down strategy
func (p *Position) GetFourthDownOption() FourthDownOption {
	return p.ensureMachine().GetFourthDownOption()
}

// CanPunt returns true if punt is still allowed
func (p *Position) CanPunt() bool {
	return p.ensureMachine().CanPunt()
}

// ExecutePunt performs punt operation
func (p *Position) ExecutePunt() error {
	if err := p.ensureMachine().ExecutePunt(); err != nil {
		return fmt.Errorf("position %s punt failed: %w", p.ID, err)
	}
	// Update the canonical persisted state to match the StateMachine
	p.State = p.StateMachine.GetCurrentState()
	return nil
}
