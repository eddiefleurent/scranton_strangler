package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
)

// Note on testing framework:
// - Using Go's built-in "testing" package.
// - If the repository already uses testify/require, these tests can be adapted to use it,
//   but we avoid introducing new dependencies per instructions.

// ---- Minimal fakes/mocks to isolate TradingCycle ----

// fakeLogger captures logs for assertions.
type fakeLogger struct {
	buf strings.Builder
}

func (l *fakeLogger) Printf(format string, v ...any) {
	l.buf.WriteString(fmt.Sprintf(format, v...))
	l.buf.WriteString("\n")
}
func (l *fakeLogger) Println(v ...any) {
	l.buf.WriteString(fmt.Sprintln(v...))
}
func (l *fakeLogger) String() string { return l.buf.String() }

// fakeStorage implements just the methods used by TradingCycle.
type fakeStorage struct {
	positions []models.Position
	addErr    error
	updErr    error
}

func (s *fakeStorage) GetCurrentPositions() []models.Position {
	// Return copy to avoid tests mutating internal slice inadvertently
	out := make([]models.Position, len(s.positions))
	copy(out, s.positions)
	return out
}
func (s *fakeStorage) AddPosition(p *models.Position) error {
	if s.addErr \!= nil {
		return s.addErr
	}
	s.positions = append(s.positions, *p)
	return nil
}
func (s *fakeStorage) UpdatePosition(p *models.Position) error {
	if s.updErr \!= nil {
		return s.updErr
	}
	for i := range s.positions {
		if s.positions[i].ID == p.ID {
			s.positions[i] = *p
			return nil
		}
	}
	return nil
}

// fakeBroker implements broker subset used.
type fakeBroker struct {
	tickSizeBySymbol map[string]float64
	marketClock      *broker.MarketClockResponse
	marketClockErr   error
	buyingPower      float64
	buyingPowerErr   error

	placeResp *broker.OrderResponse
	placeErr  error
}

func (b *fakeBroker) GetMarketClock(_ bool) (*broker.MarketClockResponse, error) {
	return b.marketClock, b.marketClockErr
}
func (b *fakeBroker) GetOptionBuyingPower() (float64, error) {
	if b.buyingPowerErr \!= nil {
		return 0, b.buyingPowerErr
	}
	return b.buyingPower, nil
}
func (b *fakeBroker) GetTickSize(symbol string) (float64, error) {
	if ts, ok := b.tickSizeBySymbol[symbol]; ok {
		return ts, nil
	}
	return 0, fmt.Errorf("no tick size for %s", symbol)
}
func (b *fakeBroker) PlaceStrangleOrder(symbol string, put, call float64, exp string, qty int, px float64, _, _ string, clientOrderID string) (*broker.OrderResponse, error) {
	_ = symbol; _ = put; _ = call; _ = exp; _ = qty; _ = px; _ = clientOrderID
	return b.placeResp, b.placeErr
}

// fakeStrategy implements strategy subset.
type fakeStrategy struct {
	entryOK       bool
	entryReason   strategy.EntryReason
	exitShould    bool
	exitReason    strategy.ExitReason
	findOrder     *strategy.StrangleOrder
	findErr       error
	currentIV     float64
	currentVal    float64
	currentValErr error
}

func (s *fakeStrategy) CheckEntryConditions() (bool, strategy.EntryReason) { return s.entryOK, s.entryReason }
func (s *fakeStrategy) CheckExitConditions(_ *models.Position) (bool, strategy.ExitReason) {
	return s.exitShould, s.exitReason
}
func (s *fakeStrategy) FindStrangleStrikes() (*strategy.StrangleOrder, error) { return s.findOrder, s.findErr }
func (s *fakeStrategy) GetCurrentIV() float64                                { return s.currentIV }
func (s *fakeStrategy) GetCurrentPositionValue(_ *models.Position) (float64, error) {
	return s.currentVal, s.currentValErr
}

// fakeOrderManager implements subset used by TradingCycle.
type fakeOrderManager struct {
	isTerminal bool
	isTermErr  error

	pollCalls int32
}

func (o *fakeOrderManager) PollOrderStatus(_ string, _ int, _ bool) {
	atomic.AddInt32(&o.pollCalls, 1)
}
func (o *fakeOrderManager) IsOrderTerminal(_ context.Context, _ int) (bool, error) {
	return o.isTerminal, o.isTermErr
}

// fakeRetryClient implements ClosePositionWithRetry.
type fakeRetryClient struct {
	resp *broker.OrderResponse
	err  error
}

func (c *fakeRetryClient) ClosePositionWithRetry(_ context.Context, _ *models.Position, _ float64) (*broker.OrderResponse, error) {
	return c.resp, c.err
}

// fakeConfig models only fields/methods used by TradingCycle.
type fakeConfig struct {
	Schedule struct {
		AfterHoursCheck bool
	}
	Risk struct {
		MaxPositions  int
		MaxContracts  int
	}
	Strategy struct {
		MaxNewPositionsPerCycle int
		Adjustments             struct {
			Enabled               bool
			EnableAdjustmentStub  bool
		}
		Exit struct {
			ProfitTarget float64
			StopLossPct  float64
		}
	}
	Broker struct {
		AccountID string
	}
	withinTradingHours bool
	withinErr          error
}

func (c *fakeConfig) IsWithinTradingHours(_ time.Time) (bool, error) {
	return c.withinTradingHours, c.withinErr
}

// fakeBot aggregates all dependencies used by TradingCycle.
type fakeBot struct {
	broker       *fakeBroker
	storage      *fakeStorage
	strategy     *fakeStrategy
	orderManager *fakeOrderManager
	retryClient  *fakeRetryClient
	config       *fakeConfig
	logger       *log.Logger
	logSink      *fakeLogger
	ctx          context.Context
	nyLocation   *time.Location
}

// NewFakeBot constructs a bot with minimal viable defaults.
func NewFakeBot() *fakeBot {
	sink := &fakeLogger{}
	logger := log.New(sink, "", 0)
	cfg := &fakeConfig{}
	return &fakeBot{
		broker:       &fakeBroker{tickSizeBySymbol: map[string]float64{}},
		storage:      &fakeStorage{},
		strategy:     &fakeStrategy{},
		orderManager: &fakeOrderManager{},
		retryClient:  &fakeRetryClient{},
		config:       cfg,
		logger:       logger,
		logSink:      sink,
		ctx:          context.Background(),
	}
}

// Adapter to real TradingCycle Bot fields/types expected by code under test.
// We define a wrapper struct type that matches the fields TradingCycle reads.
// Using type alias is not possible without original definitions; we emulate structure
// via an anonymous struct with the same field names and method set assumptions.
type Bot struct {
	broker       *fakeBroker
	storage      *fakeStorage
	strategy     *fakeStrategy
	orderManager *fakeOrderManager
	retryClient  *fakeRetryClient
	config       *fakeConfig
	logger       *log.Logger
	ctx          context.Context
	nyLocation   *time.Location
}

// NewBotFromFake maps fakeBot to Bot used by TradingCycle.
func NewBotFromFake(f *fakeBot) *Bot {
	return &Bot{
		broker:       f.broker,
		storage:      f.storage,
		strategy:     f.strategy,
		orderManager: f.orderManager,
		retryClient:  f.retryClient,
		config:       f.config,
		logger:       f.logger,
		ctx:          f.ctx,
		nyLocation:   f.nyLocation,
	}
}

// getTodaysMarketSchedule adapter method to satisfy TradingCycle.checkMarketSchedule call site.
func (b *Bot) getTodaysMarketSchedule() (*broker.MarketSchedule, error) {
	// Minimal default: open
	return &broker.MarketSchedule{
		Status: "open",
		Open:   &broker.MarketSession{Start: "09:30", End: "16:00"},
	}, nil
}

// NewReconciler adapter to satisfy NewTradingCycle() call.
// We emulate a simple reconciler that returns input unchanged.
type Reconciler struct {
}

func NewReconciler(_ *fakeBroker, _ *fakeStorage, _ *log.Logger) *Reconciler { return &Reconciler{} }
func (r *Reconciler) ReconcilePositions(p []models.Position) []models.Position { return p }

// ---- Test helpers ----

func mustNY() *time.Location {
	loc, _ := time.LoadLocation("America/New_York")
	return loc
}

func mkPosition(id, sym string, put, call float64, qty int, exp time.Time, state models.PositionState) models.Position {
	p := models.NewPosition(id, sym, put, call, exp, qty)
	_ = p.TransitionState(state, models.ConditionOrderPlaced)
	return *p
}

// ---- Tests ----

func TestCheckMarketSchedule_ClosedHolidaySkips(t *testing.T) {
	f := NewFakeBot()
	f.config.Schedule.AfterHoursCheck = false
	bot := NewBotFromFake(f)

	// Override getTodaysMarketSchedule via embedding method on our Bot
	orig := bot.getTodaysMarketSchedule
	defer func() { _ = orig }()
	bot.getTodaysMarketSchedule = func() (*broker.MarketSchedule, error) {
		return &broker.MarketSchedule{
			Status:      "closed",
			Description: "Independence Day",
		}, nil
	}

	tc := NewTradingCycle(bot)

	ran := false
	// Spy by wrapping logger
	bot.logger.SetOutput(f.logSink)

	// Run() should return early due to closed market
	tc.Run()
	logs := f.logSink.String()
	if \!strings.Contains(logs, "Market is officially CLOSED today") {
		t.Fatalf("expected holiday close log, got logs:\n%s", logs)
	}
	if strings.Contains(logs, "Starting trading cycle") {
		ran = true
	}
	if ran {
		t.Fatalf("trading cycle should not start on holiday")
	}
}

func TestCheckMarketStatus_FallbackToConfigHours(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	f.broker.marketClockErr = errors.New("api down")
	f.config.withinTradingHours = true
	tc := NewTradingCycle(bot)

	open, state := tc.checkMarketStatus()
	if \!open || state \!= "unknown" {
		t.Fatalf("expected open=true state=unknown, got open=%v state=%s", open, state)
	}
	if \!strings.Contains(f.logSink.String(), "falling back to config-based hours") {
		t.Fatalf("expected fallback log, got logs:\n%s", f.logSink.String())
	}
}

func TestShouldRunCycle_AfterHoursBehavior(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	// Market closed, no after-hours check -> skip
	f.config.Schedule.AfterHoursCheck = false
	if tc.shouldRunCycle(false, "closed") {
		t.Fatalf("expected shouldRunCycle=false when market closed and AfterHoursCheck=false")
	}

	// Market closed, after-hours check enabled -> run
	f.config.Schedule.AfterHoursCheck = true
	if \!tc.shouldRunCycle(false, "closed") {
		t.Fatalf("expected shouldRunCycle=true when market closed and AfterHoursCheck=true")
	}
}

func TestComputeEntryLimitPrice_UsesTickAndFloor(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	f.broker.tickSizeBySymbol["SPY"] = 0.05
	// credit 1.13 floors to 1.10 with tick 0.05
	got := tc.computeEntryLimitPrice("SPY", 1.13)
	want := math.Max(math.Floor(1.13/0.05)*0.05, 0.05)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("limit price mismatch: got %.2f want %.2f", got, want)
	}

	// When GetTickSize fails, fallback tick=0.01
	delete(f.broker.tickSizeBySymbol, "QQQ")
	got = tc.computeEntryLimitPrice("QQQ", 0.003)
	if math.Abs(got-0.01) > 1e-9 {
		t.Fatalf("expected min tick 0.01 on error, got %.4f", got)
	}
	if \!strings.Contains(f.logSink.String(), "Warning: Failed to get tick size for QQQ") {
		t.Fatalf("expected tick size warning log, got logs:\n%s", f.logSink.String())
	}
}

func TestExecuteEntry_SuccessPath_PlacesOrderAndSavesPosition(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	// Strategy returns a valid order
	f.strategy.findOrder = &strategy.StrangleOrder{
		Symbol:     "SPY",
		PutStrike:  400,
		CallStrike: 430,
		Expiration: time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
		Quantity:   2,
		Credit:     1.23,
		SpotPrice:  415.0,
	}
	f.broker.tickSizeBySymbol["SPY"] = 0.05
	f.config.Risk.MaxContracts = 5
	f.config.Broker.AccountID = "ABC123"

	// Broker places successfully
	f.broker.placeResp = &broker.OrderResponse{Order: broker.Order{ID: 101}}

	tc.executeEntry()

	// Position should be added and order polling started
	if len(f.storage.positions) \!= 1 {
		t.Fatalf("expected 1 position saved, got %d", len(f.storage.positions))
	}
	pos := f.storage.positions[0]
	if pos.Symbol \!= "SPY" || pos.Quantity \!= 2 {
		t.Fatalf("unexpected position saved: %+v", pos)
	}
	if pos.EntryOrderID \!= strconv.Itoa(101) {
		t.Fatalf("expected EntryOrderID=101, got %s", pos.EntryOrderID)
	}
	if atomic.LoadInt32(&f.orderManager.pollCalls) == 0 {
		t.Fatalf("expected PollOrderStatus to be invoked")
	}
	logs := f.logSink.String()
	if \!strings.Contains(logs, "Order placed successfully: 101") {
		t.Fatalf("expected order placed log, got logs:\n%s", logs)
	}
}

func TestExecuteEntry_EnforcesMaxContractsAndRejectsNonPositiveQty(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	// Order with overly large qty
	f.strategy.findOrder = &strategy.StrangleOrder{
		Symbol:     "SPY",
		PutStrike:  400,
		CallStrike: 430,
		Expiration: time.Now().AddDate(0, 0, 20).Format("2006-01-02"),
		Quantity:   100,
		Credit:     0.20,
	}
	f.config.Risk.MaxContracts = 3
	f.broker.tickSizeBySymbol["SPY"] = 0.05
	f.broker.placeResp = &broker.OrderResponse{Order: broker.Order{ID: 55}}

	tc.executeEntry()
	if \!strings.Contains(f.logSink.String(), "Position size limited to 3 contracts") {
		t.Fatalf("expected position size limited log")
	}

	// Now quantity becomes non-positive -> abort
	f.strategy.findOrder.Quantity = 0
	f.logSink.buf.Reset()
	tc.executeEntry()
	if \!strings.Contains(f.logSink.String(), "ERROR: Computed order size is non-positive") {
		t.Fatalf("expected non-positive qty abort log")
	}
}

func TestExecuteEntry_Failures_FindStrikes_ParseExpiration_PlaceOrder(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	// Find strikes failure
	f.strategy.findErr = errors.New("finder down")
	tc.executeEntry()
	if \!strings.Contains(f.logSink.String(), "Failed to find strikes") {
		t.Fatalf("expected find strikes failure log")
	}

	// Parse expiration failure
	f.strategy.findErr = nil
	f.strategy.findOrder = &strategy.StrangleOrder{
		Symbol:     "SPY",
		PutStrike:  400,
		CallStrike: 430,
		Expiration: "bad-date",
		Quantity:   1,
		Credit:     1.00,
	}
	f.broker.tickSizeBySymbol["SPY"] = 0.05
	f.logSink.buf.Reset()
	tc.executeEntry()
	if \!strings.Contains(f.logSink.String(), "Failed to parse expiration date") {
		t.Fatalf("expected bad expiration log")
	}

	// Place order failure
	f.strategy.findOrder.Expiration = time.Now().AddDate(0, 0, 15).Format("2006-01-02")
	f.broker.placeErr = errors.New("rejected")
	f.logSink.buf.Reset()
	tc.executeEntry()
	if \!strings.Contains(f.logSink.String(), "Failed to place order") {
		t.Fatalf("expected place order failure log")
	}
}

func TestCalculateMaxDebit_ProfitTarget_Time_StopLoss_Paths(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	exp := time.Now().AddDate(0, 0, 25)
	pos := mkPosition("id1", "SPY", 400, 430, 1, exp, models.StateOpen)
	pos.CreditReceived = 1.50 // abs net credit 1.50

	// Invalid config values corrected with defaults
	f.config.Strategy.Exit.ProfitTarget = 1.5 // invalid
	f.config.Strategy.Exit.StopLossPct = 0.5  // invalid

	// Profit target path
	val := tc.calculateMaxDebit(&pos, strategy.ExitReasonProfitTarget)
	if math.Abs(val-0.75) > 1e-9 { // 1.5 * (1 - 0.5 default)
		t.Fatalf("profit target debit unexpected: %.4f", val)
	}
	if \!strings.Contains(f.logSink.String(), "Invalid ProfitTarget") ||
		\!strings.Contains(f.logSink.String(), "Invalid StopLossPct") {
		t.Fatalf("expected invalid config logs")
	}

	// Time-based: when strategy value ok
	f.strategy.currentVal = 50.0 // $50 total position value
	f.strategy.currentValErr = nil
	pos.Quantity = 2
	val = tc.calculateMaxDebit(&pos, strategy.ExitReasonTime)
	want := 50.0 / (float64(2) * 100.0)
	if math.Abs(val-want) > 1e-9 {
		t.Fatalf("time-based debit mismatch: got %.4f want %.4f", val, want)
	}

	// StopLoss using current value error -> fallback to sl% of abs credit
	f.strategy.currentValErr = errors.New("na")
	pos.Quantity = 1
	f.config.Strategy.Exit.StopLossPct = 2.5 // valid
	val = tc.calculateMaxDebit(&pos, strategy.ExitReasonStopLoss)
	if math.Abs(val-3.75) > 1e-9 { // 1.5 * 2.5
		t.Fatalf("stop-loss debit mismatch: got %.4f", val)
	}
}

func TestIsPositionReadyForExit_StateMachine(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	exp := time.Now().AddDate(0, 0, 10)

	// Closed -> false
	closed := mkPosition("c1", "SPY", 400, 430, 1, exp, models.StateClosed)
	if tc.isPositionReadyForExit(&closed) {
		t.Fatalf("closed position should not be ready for exit")
	}

	// Open -> true
	open := mkPosition("o1", "SPY", 400, 430, 1, exp, models.StateOpen)
	if \!tc.isPositionReadyForExit(&open) {
		t.Fatalf("open should be ready")
	}

	// Adjusting with no ExitOrderID -> true
	adj := mkPosition("a1", "SPY", 400, 430, 1, exp, models.StateAdjusting)
	adj.ExitOrderID = ""
	if \!tc.isPositionReadyForExit(&adj) {
		t.Fatalf("adjusting without exit order should be allowed")
	}

	// Adjusting with invalid ExitOrderID -> false (strconv error)
	adj.ExitOrderID = "bad"
	if tc.isPositionReadyForExit(&adj) {
		t.Fatalf("invalid ExitOrderID should block exit")
	}

	// Adjusting with non-terminal active order -> false
	adj.ExitOrderID = "123"
	f.orderManager.isTerminal = false
	if tc.isPositionReadyForExit(&adj) {
		t.Fatalf("active non-terminal order should block exit")
	}

	// Adjusting with terminal order -> clears and returns true
	f.orderManager.isTerminal = true
	if \!tc.isPositionReadyForExit(&adj) {
		t.Fatalf("terminal prior order should allow re-attempt")
	}
	if adj.ExitOrderID \!= "" || adj.ExitReason \!= "" {
		t.Fatalf("expected exit fields cleared")
	}
}

func TestExecuteExit_SuccessfulClosePlacesOrderAndUpdatesPosition(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	exp := time.Now().AddDate(0, 0, 18)
	pos := mkPosition("x1", "SPY", 400, 430, 1, exp, models.StateOpen)
	pos.CreditReceived = 1.00

	// ensure tick size fallback path and ceil behavior
	delete(f.broker.tickSizeBySymbol, "SPY")
	f.retryClient.resp = &broker.OrderResponse{Order: broker.Order{ID: 909}}

	tc.executeExit(&pos, strategy.ExitReasonProfitTarget)

	if pos.ExitOrderID \!= "909" {
		t.Fatalf("expected ExitOrderID set, got %s", pos.ExitOrderID)
	}
	if atomic.LoadInt32(&f.orderManager.pollCalls) == 0 {
		t.Fatalf("expected PollOrderStatus invoked for exit")
	}
	logs := f.logSink.String()
	if \!strings.Contains(logs, "Close order placed for position") {
		t.Fatalf("expected close order placed log")
	}
}

func TestCheckEntryConditions_GuardsOnMaxPositionsAndBuyingPower(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	// Already at max positions -> return
	f.config.Risk.MaxPositions = 1
	now := time.Now().AddDate(0, 0, 25)
	f.storage.positions = []models.Position{
		mkPosition("p1", "SPY", 400, 430, 1, now, models.StateOpen),
	}
	f.logSink.buf.Reset()
	tc.checkEntryConditions(f.storage.GetCurrentPositions())
	if \!strings.Contains(f.logSink.String(), "Maximum positions (1) reached") {
		t.Fatalf("expected max positions guard log")
	}

	// Buying power insufficient
	f.config.Risk.MaxPositions = 5
	f.config.Strategy.MaxNewPositionsPerCycle = 2
	f.broker.buyingPower = 900 // insufficient
	f.strategy.entryOK = true
	f.logSink.buf.Reset()
	tc.checkEntryConditions([]models.Position{})
	if \!strings.Contains(f.logSink.String(), "Insufficient buying power") {
		t.Fatalf("expected insufficient buying power log")
	}
}

func TestCheckExitConditions_DelegatesToStrategyAndExecutes(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	exp := time.Now().AddDate(0, 0, 30)
	pos := mkPosition("e1", "SPY", 400, 430, 1, exp, models.StateOpen)
	f.storage.positions = []models.Position{pos}

	// Configure to allow immediate exit: retryClient returns success
	f.retryClient.resp = &broker.OrderResponse{Order: broker.Order{ID: 777}}
	f.orderManager.isTerminal = true // for subsequent readiness if needed
	f.strategy.exitShould = true
	f.strategy.exitReason = strategy.ExitReasonProfitTarget

	tc.checkExitConditions(f.storage.GetCurrentPositions())

	if \!strings.Contains(f.logSink.String(), "Exit signal for position") {
		t.Fatalf("expected exit signal log")
	}
}

func TestRun_EndToEnd_HappyPath(t *testing.T) {
	f := NewFakeBot()
	bot := NewBotFromFake(f)
	tc := NewTradingCycle(bot)

	// Market open via real-time status
	f.broker.marketClock = &broker.MarketClockResponse{Clock: broker.MarketClock{State: "open"}}

	// No positions to start
	f.storage.positions = []models.Position{}

	// Entry will pass checks
	f.config.Risk.MaxPositions = 5
	f.config.Strategy.MaxNewPositionsPerCycle = 1
	f.broker.buyingPower = 5000
	f.strategy.entryOK = true
	f.strategy.findOrder = &strategy.StrangleOrder{
		Symbol:     "SPY",
		PutStrike:  400,
		CallStrike: 430,
		Expiration: time.Now().AddDate(0, 0, 45).Format("2006-01-02"),
		Quantity:   1,
		Credit:     1.10,
		SpotPrice:  415.0,
	}
	f.broker.tickSizeBySymbol["SPY"] = 0.05
	f.broker.placeResp = &broker.OrderResponse{Order: broker.Order{ID: 2024}}

	tc.Run()

	if len(f.storage.positions) \!= 1 {
		t.Fatalf("expected one new position after Run, got %d", len(f.storage.positions))
	}
	if \!strings.Contains(f.logSink.String(), "Trading cycle complete") {
		t.Fatalf("expected completion log, got logs:\n%s", f.logSink.String())
	}
}