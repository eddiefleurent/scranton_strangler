package models

import (
	"fmt"
	"time"
)

type Position struct {
	ID             string       `json:"id"`
	Symbol         string       `json:"symbol"`
	PutStrike      float64      `json:"put_strike"`
	CallStrike     float64      `json:"call_strike"`
	Expiration     time.Time    `json:"expiration"`
	Quantity       int          `json:"quantity"`
	CreditReceived float64      `json:"credit_received"`
	EntryDate      time.Time    `json:"entry_date"`
	EntryIVR       float64      `json:"entry_ivr"`
	EntrySpot      float64      `json:"entry_spot"`
	CurrentPnL     float64      `json:"current_pnl"`
	DTE            int          `json:"dte"`
	Adjustments    []Adjustment `json:"adjustments"`
	
	// State machine for position management
	StateMachine   *StateMachine `json:"state_machine"`
}

type Adjustment struct {
	Date        time.Time `json:"date"`
	Type        string    `json:"type"` // "roll_put", "roll_call", "straddle", "inverted"
	OldStrike   float64   `json:"old_strike"`
	NewStrike   float64   `json:"new_strike"`
	Credit      float64   `json:"credit"`
	Description string    `json:"description"`
}

func (p *Position) CalculateDTE() int {
	return int(time.Until(p.Expiration).Hours() / 24)
}

func (p *Position) GetTotalCredit() float64 {
	total := p.CreditReceived
	for _, adj := range p.Adjustments {
		total += adj.Credit
	}
	return total
}

func (p *Position) ProfitPercent() float64 {
	if p.CreditReceived == 0 {
		return 0
	}
	return (p.CurrentPnL / p.CreditReceived) * 100
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
		EntryDate:    time.Now(),
		Adjustments:  make([]Adjustment, 0),
		StateMachine: NewStateMachine(),
	}
}

// TransitionState moves the position to a new state
func (p *Position) TransitionState(to PositionState, condition string) error {
	if p.StateMachine == nil {
		p.StateMachine = NewStateMachine()
	}
	
	err := p.StateMachine.Transition(to, condition)
	if err != nil {
		return fmt.Errorf("position %s state transition failed: %w", p.ID, err)
	}
	
	return nil
}

// GetCurrentState returns the current state
func (p *Position) GetCurrentState() PositionState {
	if p.StateMachine == nil {
		p.StateMachine = NewStateMachine()
	}
	return p.StateMachine.GetCurrentState()
}

// IsInManagement returns true if position is in football management states
func (p *Position) IsInManagement() bool {
	if p.StateMachine == nil {
		return false
	}
	return p.StateMachine.IsManagementState()
}

// GetManagementPhase returns which "down" we're in (1-4)
func (p *Position) GetManagementPhase() int {
	if p.StateMachine == nil {
		return 0
	}
	return p.StateMachine.GetManagementPhase()
}

// CanAdjust returns true if more adjustments are allowed
func (p *Position) CanAdjust() bool {
	if p.StateMachine == nil {
		return true
	}
	return p.StateMachine.CanAdjust()
}

// CanRoll returns true if time rolls are still allowed
func (p *Position) CanRoll() bool {
	if p.StateMachine == nil {
		return true
	}
	return p.StateMachine.CanRoll()
}

// ValidateState ensures the position state is consistent
func (p *Position) ValidateState() error {
	if p.StateMachine == nil {
		p.StateMachine = NewStateMachine()
	}
	
	// Validate state machine consistency
	if err := p.StateMachine.ValidateStateConsistency(); err != nil {
		return fmt.Errorf("position %s state validation failed: %w", p.ID, err)
	}
	
	// Validate position data consistency with state
	currentState := p.StateMachine.GetCurrentState()
	
	// Check if position data is consistent with state
	switch currentState {
	case StateIdle:
		// For new positions, allow entry date but zero credit
		if p.CreditReceived > 0 {
			return fmt.Errorf("position %s: should not have credit in state %s", p.ID, currentState)
		}
	case StateOpen, StateFirstDown, StateSecondDown, StateThirdDown, StateFourthDown:
		if p.CreditReceived <= 0 || p.EntryDate.IsZero() {
			return fmt.Errorf("position %s: missing position data for state %s", p.ID, currentState)
		}
	case StateClosed:
		if p.CreditReceived <= 0 {
			return fmt.Errorf("position %s: closed position should have credit received", p.ID)
		}
	}
	
	return nil
}

// GetStateDescription returns a human-readable state description
func (p *Position) GetStateDescription() string {
	if p.StateMachine == nil {
		return "State machine not initialized"
	}
	return p.StateMachine.GetStateDescription()
}

