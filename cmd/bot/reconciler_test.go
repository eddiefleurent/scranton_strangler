package main

import (
	"bytes"
	"errors"
	"log"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/google/uuid"
)

//
// Test doubles (minimal fakes) for broker.Broker and storage.Interface
//

type fakeBroker struct {
	positions []broker.PositionItem
	err       error
}

func (f *fakeBroker) GetPositions() ([]broker.PositionItem, error) {
	return f.positions, f.err
}

type fakeStorage struct {
	closedCalls []struct {
		id     string
		pnl    float64
		reason string
	}
	addedPositions []*models.Position
	closeErrByID   map[string]error
	addErr         error
}

// Methods used by Reconciler
func (s *fakeStorage) ClosePositionByID(id string, finalPnL float64, reason string) error {
	if s.closeErrByID \!= nil {
		if err, ok := s.closeErrByID[id]; ok {
			return err
		}
	}
	s.closedCalls = append(s.closedCalls, struct {
		id     string
		pnl    float64
		reason string
	}{id: id, pnl: finalPnL, reason: reason})
	return nil
}
func (s *fakeStorage) AddPosition(p *models.Position) error {
	if s.addErr \!= nil {
		return s.addErr
	}
	s.addedPositions = append(s.addedPositions, p)
	return nil
}

// Satisfy the full storage.Interface with no-ops if extra methods exist.
// Add common placeholders to avoid compile breakage if interface expands.
func (s *fakeStorage) GetPositions() ([]models.Position, error)                         { return nil, nil }
func (s *fakeStorage) UpdatePosition(_ *models.Position) error                          { return nil }
func (s *fakeStorage) GetPositionByID(_ string) (*models.Position, error)               { return nil, nil }
func (s *fakeStorage) DeletePositionByID(_ string) error                                { return nil }
func (s *fakeStorage) ListOpenPositions() ([]models.Position, error)                    { return nil, nil }
func (s *fakeStorage) ClosePosition(_ *models.Position, _ float64, _ string) error      { return nil }
func (s *fakeStorage) HealthCheck() error                                               { return nil }
func (s *fakeStorage) BeginTx() (storage.Tx, error)                                     { return nil, nil }
func (s *fakeStorage) WithTx(_ storage.Tx) storage.Interface                             { return s }
func (s *fakeStorage) CommitTx(_ storage.Tx) error                                      { return nil }
func (s *fakeStorage) RollbackTx(_ storage.Tx) error                                    { return nil }

// Provide a minimal Tx impl in case storage.Interface requires it.
type fakeTx struct{}
func (fakeTx) Done() bool { return true }

//
// Helpers
//

func newLoggerBuffer() (*log.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return log.New(&buf, "", 0), &buf
}

func mustPos(t *testing.T, symbol string, put, call float64, exp string, qty int) models.Position {
	t.Helper()
	id := uuid.New().String()
	expTime, err := time.Parse("2006-01-02", exp)
	if err \!= nil {
		t.Fatalf("bad exp: %v", err)
	}
	p := models.NewPosition(id, symbol, put, call, expTime, qty)
	_ = p.TransitionState(models.StateOpen, "test")
	return *p
}

//
// Unit tests for helpers
//

func Test_parseOptionSymbol(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		wantK   float64
		wantTyp string
		wantErr bool
	}{
		{"valid call", "SPY240315C00610000", 610.000, "C", false},
		{"valid put", "SPY251231P00450000", 450.000, "P", false},
		{"no type", "SPY240315X00610000", 0, "", true},
		{"short", "SPY24C", 0, "", true},
		{"bad strike", "SPY240315C00X10000", 0, "", true},
		{"type near end too short for strike", "SPY240315C001", 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotK, gotT, err := parseOptionSymbol(tt.symbol)
			if (err \!= nil) \!= tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if math.Abs(gotK-tt.wantK) > 1e-6 {
				t.Fatalf("strike=%.3f want=%.3f", gotK, tt.wantK)
			}
			if gotT \!= tt.wantTyp {
				t.Fatalf("type=%s want=%s", gotT, tt.wantTyp)
			}
		})
	}
}

func Test_extractUnderlyingFromSymbol(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"SPY", "SPY"},
		{"SPY240315C00610000", "SPY"},
		{"QQQ250101P00400000", "QQQ"},
		{"AAPL260101C00200000", "AAPL"},
		// Fallback when no 6-digit date found
		{"INVALIDSYM", "INVALIDSYM"},
	}
	for _, tt := range tests {
		if got := extractUnderlyingFromSymbol(tt.in); got \!= tt.want {
			t.Fatalf("underlying(%q)=%q want=%q", tt.in, got, tt.want)
		}
	}
}

func Test_extractExpirationFromSymbol(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"SPY240315C00610000", "2024-03-15"},
		{"QQQ250101P00400000", "2025-01-01"},
		{"SPY", ""},
		{"FOO991231C00100000", "2099-12-31"},
	}
	for _, tt := range tests {
		if got := extractExpirationFromSymbol(tt.in); got \!= tt.want {
			t.Fatalf("expiration(%q)=%q want=%q", tt.in, got, tt.want)
		}
	}
}

func Test_identifyStranglesFromPositions(t *testing.T) {
	exp := "2025-09-19"
	makePos := func(sym string, qty int) broker.PositionItem { return broker.PositionItem{Symbol: sym, Quantity: float64(qty)} }

	positions := []broker.PositionItem{
		makePos("SPY250919C00500000", -1),
		makePos("SPY250919P00450000", -1),
		// mismatched quantities should not pair
		makePos("SPY250919C00600000", -2),
		makePos("SPY250919P00550000", -1),
		// different expiration shouldn't be considered inside this call
		makePos("SPY250920C00500000", -1),
		makePos("SPY250920P00450000", -1),
		// invalid symbol ignored
		makePos("SPY250919X00500000", -1),
	}

	got := identifyStranglesFromPositions(positions, exp)
	// Expect only the 500/450 pair for 250919 with equal qty 1
	if len(got) == 0 {
		t.Fatalf("expected at least one strangle, got none")
	}
	found := false
	for _, s := range got {
		if s.expiration == exp && math.Abs(s.callStrike-500) < 0.01 && math.Abs(s.putStrike-450) < 0.01 && s.quantity == 1 && s.symbol == "SPY" {
			found = true
		}
	}
	if \!found {
		t.Fatalf("expected SPY 500C/450P x1 for %s in %+v", exp, got)
	}
}

func Test_strangleMatches(t *testing.T) {
	exp := "2024-03-15"
	pos := mustPos(t, "SPY", 450, 500, exp, 2)

	ok := strangleMatches(pos, orphanedStrangle{
		putStrike:  450,
		callStrike: 500,
		expiration: exp,
		quantity:   2,
		symbol:     "SPY",
	})
	if \!ok {
		t.Fatalf("expected match")
	}

	badQty := strangleMatches(pos, orphanedStrangle{
		putStrike:  450, callStrike: 500, expiration: exp, quantity: 1, symbol: "SPY",
	})
	if badQty {
		t.Fatalf("expected quantity mismatch to fail")
	}

	badExp := strangleMatches(pos, orphanedStrangle{
		putStrike:  450, callStrike: 500, expiration: "2024-03-22", quantity: 2, symbol: "SPY",
	})
	if badExp {
		t.Fatalf("expected expiration mismatch to fail")
	}
}

//
// Unit tests for isPositionOpenInBroker
//

func Test_isPositionOpenInBroker(t *testing.T) {
	logger, _ := newLoggerBuffer()
	rec := &Reconciler{logger: logger}

	exp := "2024-03-15"
	pos := mustPos(t, "SPY", 450, 500, exp, 1)

	okPositions := []broker.PositionItem{
		{Symbol: "SPY240315C00500000", Quantity: -1},
		{Symbol: "SPY240315P00450000", Quantity: -1},
	}
	if \!rec.isPositionOpenInBroker(&pos, okPositions) {
		t.Fatalf("expected open with both legs present")
	}

	// Missing put leg
	missingPut := []broker.PositionItem{
		{Symbol: "SPY240315C00500000", Quantity: -1},
	}
	if rec.isPositionOpenInBroker(&pos, missingPut) {
		t.Fatalf("expected false when one leg missing")
	}

	// Wrong strikes
	wrongStrike := []broker.PositionItem{
		{Symbol: "SPY240315C00510000", Quantity: -1},
		{Symbol: "SPY240315P00440000", Quantity: -1},
	}
	if rec.isPositionOpenInBroker(&pos, wrongStrike) {
		t.Fatalf("expected false for mismatched strikes")
	}

	// Different underlying
	diffUly := []broker.PositionItem{
		{Symbol: "QQQ240315C00500000", Quantity: -1},
		{Symbol: "QQQ240315P00450000", Quantity: -1},
	}
	if rec.isPositionOpenInBroker(&pos, diffUly) {
		t.Fatalf("expected false for different underlying")
	}

	// Different expiration
	diffExp := []broker.PositionItem{
		{Symbol: "SPY240322C00500000", Quantity: -1},
		{Symbol: "SPY240322P00450000", Quantity: -1},
	}
	if rec.isPositionOpenInBroker(&pos, diffExp) {
		t.Fatalf("expected false for different expiration")
	}
}

//
// Unit tests for createRecoveryPosition
//

func Test_createRecoveryPosition_Success(t *testing.T) {
	logger, _ := newLoggerBuffer()
	rec := &Reconciler{logger: logger}

	orphan := orphanedStrangle{
		putStrike:  450,
		callStrike: 500,
		expiration: "2024-03-15",
		quantity:   2,
		symbol:     "SPY",
	}
	got := rec.createRecoveryPosition(orphan)
	if got == nil {
		t.Fatalf("expected position")
	}
	if got.Symbol \!= "SPY" || got.Quantity \!= 2 {
		t.Fatalf("unexpected position fields: %+v", got)
	}
	if got.GetCurrentState() \!= models.StateOpen {
		t.Fatalf("expected recovered position to be StateOpen")
	}
	if got.EntryDate.IsZero() {
		t.Fatalf("expected EntryDate to be set")
	}
}

func Test_createRecoveryPosition_BadDate(t *testing.T) {
	logger, _ := newLoggerBuffer()
	rec := &Reconciler{logger: logger}

	orphan := orphanedStrangle{putStrike: 450, callStrike: 500, expiration: "bad-date", quantity: 1, symbol: "SPY"}
	if got := rec.createRecoveryPosition(orphan); got \!= nil {
		t.Fatalf("expected nil for bad date")
	}
}

//
// Unit tests for findOrphanedStrangles (integration of several helpers)
//

func Test_findOrphanedStrangles_FiltersTrackedAndNonSPY(t *testing.T) {
	logger, _ := newLoggerBuffer()
	rec := &Reconciler{logger: logger}

	// Broker has one SPY strangle and one QQQ strangle
	brokerPositions := []broker.PositionItem{
		{Symbol: "SPY250919C00500000", Quantity: -1},
		{Symbol: "SPY250919P00450000", Quantity: -1},
		{Symbol: "QQQ250919C00350000", Quantity: -1},
		{Symbol: "QQQ250919P00300000", Quantity: -1},
	}
	// Active positions already tracking the SPY pair
	active := []models.Position{
		mustPos(t, "SPY", 450, 500, "2025-09-19", 1),
	}

	orphaned := rec.findOrphanedStrangles(brokerPositions, active)
	if len(orphaned) \!= 0 {
		t.Fatalf("expected no orphaned SPY strangles when already tracked; got %+v", orphaned)
	}
}

func Test_findOrphanedStrangles_DetectsMissing(t *testing.T) {
	logger, _ := newLoggerBuffer()
	rec := &Reconciler{logger: logger}

	brokerPositions := []broker.PositionItem{
		{Symbol: "SPY251220C00480000", Quantity: -3},
		{Symbol: "SPY251220P00430000", Quantity: -3},
	}
	active := []models.Position{} // none tracked

	orphaned := rec.findOrphanedStrangles(brokerPositions, active)
	if len(orphaned) \!= 1 {
		t.Fatalf("expected 1 orphaned strangle, got %d", len(orphaned))
	}
	if orphaned[0].quantity \!= 3 || math.Abs(orphaned[0].callStrike-480) > 0.01 || math.Abs(orphaned[0].putStrike-430) > 0.01 {
		t.Fatalf("unexpected orphaned details: %+v", orphaned[0])
	}
}

//
// Unit tests for ReconcilePositions (end-to-end behavior)
//

func Test_ReconcilePositions_BrokerErrorReturnsUnchanged(t *testing.T) {
	logger, _ := newLoggerBuffer()
	st := &fakeStorage{}
	br := &fakeBroker{err: errors.New("broker down")}
	rec := NewReconciler(br, st, logger)

	stored := []models.Position{
		mustPos(t, "SPY", 450, 500, "2024-03-15", 1),
	}
	got := rec.ReconcilePositions(stored)
	if len(got) \!= 1 {
		t.Fatalf("expected unchanged positions on broker error, got %d", len(got))
	}
}

func Test_ReconcilePositions_ManualCloseWithComputedPnL(t *testing.T) {
	logger, _ := newLoggerBuffer()
	st := &fakeStorage{}
	br := &fakeBroker{positions: []broker.PositionItem{}} // nothing open in broker
	rec := NewReconciler(br, st, logger)

	// Stored active position with zero CurrentPnL; expect computed from credit*qty*100
	p := mustPos(t, "SPY", 450, 500, "2024-03-15", 2)
	p.CurrentPnL = 0
	p.CreditReceived = 1.25 // $1.25 credit
	p.ID = "pos-1"

	got := rec.ReconcilePositions([]models.Position{p})
	if len(got) \!= 0 {
		t.Fatalf("expected no active positions after manual close, got %d", len(got))
	}
	if len(st.closedCalls) \!= 1 {
		t.Fatalf("expected one close call")
	}
	close := st.closedCalls[0]
	if close.id \!= "pos-1" || close.reason \!= "manual_close" {
		t.Fatalf("unexpected close details: %+v", close)
	}
	wantPnL := math.Abs(p.CreditReceived) * float64(p.Quantity) * 100
	if math.Abs(close.pnl-wantPnL) > 1e-6 {
		t.Fatalf("pnl=%.2f want=%.2f", close.pnl, wantPnL)
	}
}

func Test_ReconcilePositions_ManualCloseUsesCurrentPnLWhenNonZero(t *testing.T) {
	logger, _ := newLoggerBuffer()
	st := &fakeStorage{}
	br := &fakeBroker{positions: []broker.PositionItem{}}
	rec := NewReconciler(br, st, logger)

	p := mustPos(t, "SPY", 450, 500, "2024-03-15", 1)
	p.CurrentPnL = 321.45
	p.CreditReceived = 0.50
	p.ID = "pos-2"

	_ = rec.ReconcilePositions([]models.Position{p})
	if len(st.closedCalls) \!= 1 {
		t.Fatalf("expected one close call")
	}
	if math.Abs(st.closedCalls[0].pnl-321.45) > 1e-6 {
		t.Fatalf("expected to use CurrentPnL when non-zero")
	}
}

func Test_ReconcilePositions_CloseErrorKeepsActive(t *testing.T) {
	logger, _ := newLoggerBuffer()
	st := &fakeStorage{closeErrByID: map[string]error{"pos-3": errors.New("db error")}}
	br := &fakeBroker{positions: []broker.PositionItem{}}
	rec := NewReconciler(br, st, logger)

	p := mustPos(t, "SPY", 450, 500, "2024-03-15", 1)
	p.ID = "pos-3"

	got := rec.ReconcilePositions([]models.Position{p})
	if len(got) \!= 1 {
		t.Fatalf("expected position to remain active when storage close fails")
	}
}

func Test_ReconcilePositions_ActiveWhenBrokerHasBothLegs(t *testing.T) {
	logger, _ := newLoggerBuffer()
	st := &fakeStorage{}
	br := &fakeBroker{positions: []broker.PositionItem{
		{Symbol: "SPY240315C00500000", Quantity: -1},
		{Symbol: "SPY240315P00450000", Quantity: -1},
	}}
	rec := NewReconciler(br, st, logger)

	p := mustPos(t, "SPY", 450, 500, "2024-03-15", 1)
	got := rec.ReconcilePositions([]models.Position{p})
	if len(got) \!= 1 {
		t.Fatalf("expected position to stay active when open in broker")
	}
	if len(st.closedCalls) \!= 0 {
		t.Fatalf("did not expect manual close")
	}
}

func Test_ReconcilePositions_AddsRecoveryForOrphanedStrangle(t *testing.T) {
	logger, buf := newLoggerBuffer()
	st := &fakeStorage{}
	br := &fakeBroker{positions: []broker.PositionItem{
		{Symbol: "SPY250919C00500000", Quantity: -2},
		{Symbol: "SPY250919P00450000", Quantity: -2},
	}}
	rec := NewReconciler(br, st, logger)

	got := rec.ReconcilePositions([]models.Position{}) // no stored positions
	if len(got) \!= 1 {
		t.Fatalf("expected 1 active position after recovery, got %d", len(got))
	}
	if len(st.addedPositions) \!= 1 {
		t.Fatalf("expected storage.AddPosition to be called once")
	}
	// Verify some log messages emitted (sanity check)
	logs := buf.String()
	if \!strings.Contains(logs, "orphaned strangle") || \!strings.Contains(logs, "Added recovery position") {
		t.Fatalf("expected logs to mention orphaned strangle and added recovery; got logs:\n%s", logs)
	}
}

func Test_ReconcilePositions_AddRecoveryAddFailure(t *testing.T) {
	logger, _ := newLoggerBuffer()
	st := &fakeStorage{addErr: errors.New("insert failed")}
	br := &fakeBroker{positions: []broker.PositionItem{
		{Symbol: "SPY250919C00500000", Quantity: -1},
		{Symbol: "SPY250919P00450000", Quantity: -1},
	}}
	rec := NewReconciler(br, st, logger)

	got := rec.ReconcilePositions([]models.Position{})
	// Even if AddPosition fails, function should return whatever activePositions existed (none in this case)
	if len(got) \!= 0 {
		t.Fatalf("expected 0 active positions when add fails, got %d", len(got))
	}
}