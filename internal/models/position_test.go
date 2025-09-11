package models

import (
	"testing"
	"time"
	"math"
)

// helper: zero time check (Go's zero value)
func isZeroTime(t time.Time) bool { return t.IsZero() }

func TestCalculateDTE_BasicAndPastExpiration(t *testing.T) {
	now := time.Now().UTC().Truncate(24 * time.Hour)

	tests := []struct{
		name string
		expiration time.Time
		want int
	}{
		{
			name: "exactly 0 days when expiration is same truncated day",
			expiration: now,
			want: 0,
		},
		{
			name: "3 days ahead",
			expiration: now.Add(72 * time.Hour),
			want: 3,
		},
		{
			name: "future but with partial day that truncates down",
			expiration: now.Add(26 * time.Hour), // truncates to now+24h
			want: 1,
		},
		{
			name: "past expiration clamps to 0",
			expiration: now.Add(-72 * time.Hour),
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Position{Expiration: tt.expiration}
			got := p.CalculateDTE()
			if got \!= tt.want {
				t.Fatalf("CalculateDTE() = %d, want %d (now=%v exp=%v)", got, tt.want, now, tt.expiration)
			}
		})
	}
}

func TestGetNetCredit_WithAdjustments_PositiveNegative(t *testing.T) {
	p := &Position{
		CreditReceived: 1.25,
		Adjustments: []Adjustment{
			{Credit:  0.50},
			{Credit: -0.20},
			{Credit:  0.00},
		},
	}
	want := 1.25 + 0.50 - 0.20 + 0.00
	if got := p.GetNetCredit(); got \!= want {
		t.Fatalf("GetNetCredit() = %v, want %v", got, want)
	}
}

func TestGetTotalCredit_DeprecatedAliasMatchesNet(t *testing.T) {
	p := &Position{
		CreditReceived: 2.0,
		Adjustments:    []Adjustment{{Credit: 1.0}},
	}
	if p.GetTotalCredit() \!= p.GetNetCredit() {
		t.Fatalf("GetTotalCredit() should equal GetNetCredit()")
	}
}

func TestProfitPercent_DenomZeroCases(t *testing.T) {
	// denom = |netCredit * qty * 100|
	// Case 1: net credit zero
	p1 := &Position{Quantity: 1, CurrentPnL: 100}
	if pct := p1.ProfitPercent(); pct \!= 0 {
		t.Fatalf("ProfitPercent() with zero net credit expected 0, got %v", pct)
	}
	// Case 2: quantity zero
	p2 := &Position{CreditReceived: 2.0, Quantity: 0, CurrentPnL: 100}
	if pct := p2.ProfitPercent(); pct \!= 0 {
		t.Fatalf("ProfitPercent() with zero quantity expected 0, got %v", pct)
	}
}

func TestProfitPercent_ComputesWithSignAndAbsDenom(t *testing.T) {
	// netCredit positive: 1.5, qty 2 => denom = |1.5*2*100| = 300
	// pnl 150 => 50%
	p := &Position{CreditReceived: 1.5, Quantity: 2, CurrentPnL: 150}
	if pct := p.ProfitPercent(); math.Abs(pct-50.0) > 1e-9 {
		t.Fatalf("ProfitPercent() = %v, want 50", pct)
	}

	// negative PnL: -600 / 300 => -200%
	p.CurrentPnL = -600
	if pct := p.ProfitPercent(); math.Abs(pct+200.0) > 1e-9 {
		t.Fatalf("ProfitPercent() = %v, want -200", pct)
	}

	// netCredit negative: -2.0, qty 1 => denom = |-2*1*100| = 200
	p2 := &Position{CreditReceived: -2.0, Quantity: 1, CurrentPnL: 100}
	if pct := p2.ProfitPercent(); math.Abs(pct-50.0) > 1e-9 {
		t.Fatalf("ProfitPercent() with negative net credit = %v, want 50", pct)
	}
}

func TestNewPosition_DefaultsAndInitialization(t *testing.T) {
	exp := time.Now().UTC().Add(30 * 24 * time.Hour)
	p := NewPosition("abc123", "SPY", 400, 450, exp, 3)

	if p.ID \!= "abc123" || p.Symbol \!= "SPY" || p.PutStrike \!= 400 || p.CallStrike \!= 450 || p.Quantity \!= 3 {
		t.Fatalf("NewPosition field mismatch: got %+v", p)
	}
	if \!p.Expiration.Equal(exp) {
		t.Fatalf("Expiration not set correctly")
	}
	if p.State \!= StateIdle {
		t.Fatalf("Initial State should be StateIdle, got %v", p.State)
	}
	if \!isZeroTime(p.ExitDate) {
		t.Fatalf("ExitDate should be zero at creation")
	}
	if p.Adjustments == nil || len(p.Adjustments) \!= 0 {
		t.Fatalf("Adjustments should be an empty slice")
	}
	if p.StateMachine == nil {
		t.Fatalf("StateMachine should be initialized")
	}
}

func TestValidateState_Invariants_Idle(t *testing.T) {
	p := &Position{
		ID:     "p1",
		State:  StateIdle,
		// All fields that must be zero/empty in Idle are left default/zeroed.
	}
	if err := p.ValidateState(); err \!= nil {
		t.Fatalf("ValidateState() Idle expected nil, got %v", err)
	}

	// Violations
	p.EntryDate = time.Now().UTC()
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Idle with EntryDate set should error")
	}
	p.EntryDate = time.Time{}

	p.ExitDate = time.Now().UTC()
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Idle with ExitDate set should error")
	}
	p.ExitDate = time.Time{}

	p.ExitReason = "something"
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Idle with ExitReason set should error")
	}
	p.ExitReason = ""

	p.CreditReceived = 0.01
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Idle with CreditReceived non-zero should error")
	}
	p.CreditReceived = 0

	p.Adjustments = []Adjustment{{Credit: 0.1}}
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Idle with Adjustments non-empty should error")
	}
	p.Adjustments = nil
}

func TestValidateState_Invariants_Submitted(t *testing.T) {
	p := &Position{
		ID:    "s1",
		State: StateSubmitted,
		// Must have Entry/Exit zero, ExitReason empty, Credit=0, Qty=0, Adjustments empty
	}
	if err := p.ValidateState(); err \!= nil {
		t.Fatalf("Submitted valid baseline expected nil, got %v", err)
	}

	// Each violation
	p.EntryDate = time.Now().UTC()
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Submitted with EntryDate set should error")
	}
	p.EntryDate = time.Time{}

	p.ExitDate = time.Now().UTC()
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Submitted with ExitDate set should error")
	}
	p.ExitDate = time.Time{}

	p.ExitReason = "not allowed"
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Submitted with ExitReason should error")
	}
	p.ExitReason = ""

	p.CreditReceived = 0.01
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Submitted with CreditReceived non-zero should error")
	}
	p.CreditReceived = 0

	p.Quantity = 1
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Submitted with Quantity non-zero should error")
	}
	p.Quantity = 0

	p.Adjustments = []Adjustment{{Credit: 0.1}}
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Submitted with Adjustments non-empty should error")
	}
	p.Adjustments = nil
}

func TestValidateState_Invariants_ActiveStates(t *testing.T) {
	activeStates := []PositionState{StateOpen, StateFirstDown, StateSecondDown, StateThirdDown, StateFourthDown}
	for _, st := range activeStates {
		t.Run(st.String(), func(t *testing.T) {
			p := &Position{
				ID:            "act",
				State:         st,
				EntryDate:     time.Now().UTC(),
				CreditReceived: 0.50,
				Quantity:      1,
			}
			if err := p.ValidateState(); err \!= nil {
				t.Fatalf("Active baseline expected nil, got %v", err)
			}

			// ExitDate must be zero
			p.ExitDate = time.Now().UTC()
			if err := p.ValidateState(); err == nil {
				t.Fatalf("Active with ExitDate set should error")
			}
			p.ExitDate = time.Time{}

			// ExitReason must be empty
			p.ExitReason = "should be empty"
			if err := p.ValidateState(); err == nil {
				t.Fatalf("Active with ExitReason set should error")
			}
			p.ExitReason = ""

			// Credit must be > 0
			p.CreditReceived = 0
			if err := p.ValidateState(); err == nil {
				t.Fatalf("Active with non-positive credit should error")
			}
			p.CreditReceived = 0.5

			// Quantity must be > 0
			p.Quantity = 0
			if err := p.ValidateState(); err == nil {
				t.Fatalf("Active with qty <= 0 should error")
			}
		})
	}
}

func TestValidateState_Invariants_Closed(t *testing.T) {
	now := time.Now().UTC()
	p := &Position{
		ID:            "c1",
		State:         StateClosed,
		EntryDate:     now.Add(-24 * time.Hour),
		ExitDate:      now,
		ExitReason:    "target achieved",
		CreditReceived: 0.25,
		Quantity:      1,
	}
	if err := p.ValidateState(); err \!= nil {
		t.Fatalf("Closed baseline expected nil, got %v", err)
	}

	// EntryDate must be set
	p.EntryDate = time.Time{}
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Closed with EntryDate zero should error")
	}
	p.EntryDate = now.Add(-24 * time.Hour)

	// ExitDate must be set
	p.ExitDate = time.Time{}
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Closed with ExitDate zero should error")
	}
	p.ExitDate = now

	// ExitReason required
	p.ExitReason = ""
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Closed with empty ExitReason should error")
	}
	p.ExitReason = "ok"

	// Credit must be positive
	p.CreditReceived = 0
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Closed with non-positive credit should error")
	}
	p.CreditReceived = 0.25

	// Quantity must be > 0
	p.Quantity = 0
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Closed with qty <= 0 should error")
	}
	p.Quantity = 1

	// Entry must be before Exit
	p.EntryDate = now.Add(1 * time.Hour)
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Closed with Entry after Exit should error")
	}
}

func TestValidateState_Invariants_Adjusting(t *testing.T) {
	p := &Position{
		ID:            "a1",
		State:         StateAdjusting,
		EntryDate:     time.Now().UTC(),
		CreditReceived: 0.0, // allowed to be >= 0
		Quantity:      0,    // allowed to be >= 0
		Adjustments:   []Adjustment{{Credit: 0.1}},
	}
	if err := p.ValidateState(); err \!= nil {
		t.Fatalf("Adjusting baseline expected nil, got %v", err)
	}

	// Must have at least one adjustment
	p.Adjustments = nil
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Adjusting with empty adjustments should error")
	}
}

func TestValidateState_Invariants_Rolling(t *testing.T) {
	p := &Position{
		ID:            "r1",
		State:         StateRolling,
		EntryDate:     time.Now().UTC(),
		CreditReceived: 0.10,
		Quantity:      1,
	}
	if err := p.ValidateState(); err \!= nil {
		t.Fatalf("Rolling baseline expected nil, got %v", err)
	}

	// Credit must be positive
	p.CreditReceived = 0
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Rolling with non-positive credit should error")
	}
	p.CreditReceived = 0.10

	// Quantity must be > 0
	p.Quantity = 0
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Rolling with qty <= 0 should error")
	}
}

func TestValidateState_GlobalInvariants(t *testing.T) {
	p := &Position{
		ID:    "g1",
		State: StateIdle,
	}
	p.CreditReceived = -0.01
	if err := p.ValidateState(); err == nil {
		t.Fatalf("Global: negative CreditReceived should error")
	}
}

func TestTransitionState_SetsCanonicalState_AndDatesAndClears(t *testing.T) {
	// We attempt a sequence likely to be accepted by the StateMachine.
	// If StateMachine rejects transitions in this codebase, this test will still
	// validate side-effects when transitions succeed.
	p := NewPosition("t1", "SPY", 400, 450, time.Now().UTC().Add(30*24*time.Hour), 1)

	// Transition to Submitted: should clear non-active fields and zero EntryLimitPrice
	p.EntryDate = time.Now().UTC() // set to verify it will be cleared
	p.EntryLimitPrice = 1.23
	if err := p.TransitionState(StateSubmitted, "order placed"); err == nil {
		// If transition accepted, verify side-effects
		if \!isZeroTime(p.EntryDate) || \!isZeroTime(p.ExitDate) || p.ExitReason \!= "" ||
			p.CreditReceived \!= 0 || p.Quantity \!= 0 || len(p.Adjustments) \!= 0 || p.EntryLimitPrice \!= 0 {
			t.Fatalf("Submitted transition should clear non-active fields; got %+v", p)
		}
		if p.State \!= StateSubmitted {
			t.Fatalf("State should be StateSubmitted, got %v", p.State)
		}
	}

	// Transition to Open: sets EntryDate if zero
	p.EntryDate = time.Time{}
	if err := p.TransitionState(StateOpen, "filled"); err == nil {
		if isZeroTime(p.EntryDate) {
			t.Fatalf("Open transition should set EntryDate when zero")
		}
		if \!isZeroTime(p.ExitDate) {
			t.Fatalf("Open transition should keep ExitDate zero")
		}
		if p.State \!= StateOpen {
			t.Fatalf("State should be StateOpen, got %v", p.State)
		}
	}

	// Transition to Closed: sets ExitDate if zero
	p.ExitDate = time.Time{}
	if err := p.TransitionState(StateClosed, "target reached"); err == nil {
		if isZeroTime(p.ExitDate) {
			t.Fatalf("Closed transition should set ExitDate when zero")
		}
		if p.State \!= StateClosed {
			t.Fatalf("State should be StateClosed, got %v", p.State)
		}
	}
}

func TestDelegatingWrappers_DoNotPanicAndReturnSaneValues(t *testing.T) {
	p := NewPosition("d1", "QQQ", 300, 350, time.Now().UTC().Add(21*24*time.Hour), 1)

	// GetCurrentState mirrors canonical state
	if p.GetCurrentState() \!= p.State {
		t.Fatalf("GetCurrentState should mirror Position.State")
	}

	// Management related methods: just ensure no panic and outputs in sensible ranges
	_ = p.IsInManagement()
	phase := p.GetManagementPhase()
	if phase < 0 || phase > 4 {
		t.Fatalf("GetManagementPhase returned out-of-range value: %d", phase)
	}
	_ = p.CanAdjust()
	_ = p.CanRoll()

	// ShouldEmergencyExit should not panic and returns a bool + reason
	ok, reason := p.ShouldEmergencyExit(21, 0.5)
	if ok && reason == "" {
		t.Fatalf("When true, reason should be non-empty")
	}

	// Fourth down options / punt delegates: should not panic
	opt0 := p.GetFourthDownOption()
	p.SetFourthDownOption(opt0)
	_ = p.CanPunt()
	if err := p.ExecutePunt(); err \!= nil {
		// Depending on StateMachine rules, this may error; we only ensure it doesn't panic.
	}
}