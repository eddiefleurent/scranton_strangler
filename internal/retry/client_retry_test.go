package retry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
)

// --- Test helpers ---

type fakeBroker struct {
	callCount int32

	// scripted behaviors
	// if successAfterN > 0, return errTransient for attempts < N, then success
	successAfterN int
	errTransient  error
	errPermanent  error

	// response to return on success
	resp *broker.OrderResponse
}

func (f *fakeBroker) CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string, qty int, maxDebit float64) (*broker.OrderResponse, error) {
	atomic.AddInt32(&f.callCount, 1)

	// If configured to succeed after N attempts, return transient errors until then.
	if f.successAfterN > 0 {
		if int(atomic.LoadInt32(&f.callCount)) < f.successAfterN {
			if f.errTransient != nil {
				return nil, f.errTransient
			}
			return nil, errors.New("timeout") // default transient
		}
		return f.successResponse(), nil
	}

	// If permanent error requested, return it
	if f.errPermanent != nil {
		return nil, f.errPermanent
	}

	// Otherwise return success
	return f.successResponse(), nil
}

func (f *fakeBroker) successResponse() *broker.OrderResponse {
	if f.resp != nil {
		return f.resp
	}
	// Construct a minimal plausible OrderResponse.
	// We only access Order.ID in production code logs; keep structure compatible.
	return &broker.OrderResponse{
		Order: broker.Order{
			ID: 12345,
		},
	}
}

func newTestPosition() *models.Position {
	// Fill required fields referenced by production code.
	return &models.Position{
		ID:        "pos-abc-123",
		Symbol:    "ABC",
		PutStrike: 95.0,
		CallStrike: 105.0,
		Expiration: time.Date(2025, 10, 17, 0, 0, 0, 0, time.UTC),
		Quantity:   1,
	}
}

// makeClient builds a Client with controllable timing and a buffer-backed logger.
func makeClient(t *testing.T, br broker.Broker, cfg Config) (*Client, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	l := log.New(&buf, "", 0)
	c := NewClient(br, l, cfg)
	return c, &buf
}

// --- Tests ---

func TestNewClient_ConfigSanitizationAndDefaults(t *testing.T) {
	br := &fakeBroker{}
	var buf bytes.Buffer

	// Provide bad config values to ensure sanitization to DefaultConfig
	cfg := Config{
		MaxRetries:     -1,
		InitialBackoff: 0,
		MaxBackoff:     0,
		Timeout:        0,
	}
	c := NewClient(br, nil, cfg) // nil logger => defaulted internally

	if c.broker == nil {
		t.Fatalf("expected broker to be set")
	}
	if c.logger == nil {
		t.Fatalf("expected logger to be non-nil (defaulted)")
	}
	if c.config.MaxRetries != DefaultConfig.MaxRetries {
		t.Fatalf("MaxRetries sanitized: got %d want %d", c.config.MaxRetries, DefaultConfig.MaxRetries)
	}
	if c.config.InitialBackoff != DefaultConfig.InitialBackoff {
		t.Fatalf("InitialBackoff sanitized: got %v want %v", c.config.InitialBackoff, DefaultConfig.InitialBackoff)
	}
	if c.config.MaxBackoff != DefaultConfig.MaxBackoff {
		t.Fatalf("MaxBackoff sanitized: got %v want %v", c.config.MaxBackoff, DefaultConfig.MaxBackoff)
	}
	if c.config.Timeout != DefaultConfig.Timeout {
		t.Fatalf("Timeout sanitized: got %v want %v", c.config.Timeout, DefaultConfig.Timeout)
	}

	// Also ensure explicit non-nil logger is honored
	l := log.New(&buf, "", 0)
	c2 := NewClient(br, l)
	if c2.logger != l {
		t.Fatalf("expected provided logger to be used")
	}
}

func TestIsTransientError_Patterns(t *testing.T) {
	c, _ := makeClient(t, &fakeBroker{}, DefaultConfig)

	cases := []struct{
		name string
		err error
		want bool
	}{
		{"nil", nil, false},
		{"timeout", errors.New("request TIMEOUT while processing"), true},
		{"conn refused", errors.New("connection refused by target"), true},
		{"conn reset", errors.New("read: connection reset by peer"), true},
		{"temporary failure", errors.New("temporary failure in name resolution"), true},
		{"server error", errors.New("internal server error"), true},
		{"rate limit", errors.New("rate limit exceeded"), true},
		{"429", errors.New("HTTP 429 Too Many Requests"), true},
		{"502", errors.New("502 bad gateway"), true},
		{"503", errors.New("Service Unavailable (503)"), true},
		{"504", errors.New("504 Gateway Timeout"), true},
		{"network", errors.New("network unreachable"), true},
		{"dns", errors.New("dns lookup failed"), true},
		{"tcp", errors.New("tcp handshake failed"), true},
		{"non-transient", errors.New("validation failed: credit check"), false},
		{"empty string", errors.New(""), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.isTransientError(tc.err)
			if got != tc.want {
				t.Fatalf("isTransientError(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestCalculateNextBackoff_GeneralBehavior(t *testing.T) {
	cfg := Config{
		MaxRetries:     2,
		InitialBackoff: 4 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Timeout:        1 * time.Second,
	}
	c, _ := makeClient(t, &fakeBroker{}, cfg)

	// Case 1: multiply by 1.5 within max, with jitter in [0, backoff/4)
	next := c.calculateNextBackoff(4 * time.Millisecond) // base = 6ms, jitter in [0, 1ms)
	if next < 6*time.Millisecond || next >= 7*time.Millisecond {
		t.Fatalf("unexpected next backoff: got %v, expected [6ms,7ms)", next)
	}

	// Case 2: cap to MaxBackoff before jitter, then allow jitter up to MaxBackoff/4
	next2 := c.calculateNextBackoff(8 * time.Millisecond) // base=12ms -> capped at 10ms; jitter in [0, 2ms)
	if next2 < 10*time.Millisecond || next2 >= 12*time.Millisecond {
		t.Fatalf("unexpected capped next backoff: got %v, expected [10ms,12ms)", next2)
	}

	// Case 3: zero input stays zero (no jitter)
	if got := c.calculateNextBackoff(0); got != 0 {
		t.Fatalf("zero backoff expected to remain zero, got %v", got)
	}
}

func TestClosePositionWithRetry_SucceedsFirstAttempt(t *testing.T) {
	fb := &fakeBroker{
		// success immediately
	}
	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
		Timeout:        250 * time.Millisecond,
	}
	c, buf := makeClient(t, fb, cfg)

	ctx := context.Background()
	pos := newTestPosition()
	resp, err := c.ClosePositionWithRetry(ctx, pos, 1.23)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}
	if atomic.LoadInt32(&fb.callCount) != 1 {
		t.Fatalf("expected 1 broker call, got %d", fb.callCount)
	}
	if !strings.Contains(buf.String(), "Close attempt 1/") {
		t.Fatalf("expected log to contain attempt log, got: %s", buf.String())
	}
}

func TestClosePositionWithRetry_RetriesOnTransientAndThenSucceeds(t *testing.T) {
	fb := &fakeBroker{
		successAfterN: 3, // fail twice, succeed third
		errTransient:  errors.New("timeout while closing"),
	}
	cfg := Config{
		MaxRetries:     3, // allows up to 4 attempts total
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     3 * time.Millisecond,
		Timeout:        250 * time.Millisecond,
	}
	c, _ := makeClient(t, fb, cfg)

	ctx := context.Background()
	pos := newTestPosition()

	start := time.Now()
	resp, err := c.ClosePositionWithRetry(ctx, pos, 2.34)
	if err != nil {
		t.Fatalf("expected success after retries, got err: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected response after retries")
	}
	if atomic.LoadInt32(&fb.callCount) != 3 {
		t.Fatalf("expected 3 attempts, got %d", fb.callCount)
	}
	// Ensure some small wait occurred (not strict, just sanity)
	if elapsed := time.Since(start); elapsed < 2*time.Millisecond {
		t.Fatalf("expected some backoff elapsed, got %v", elapsed)
	}
}

func TestClosePositionWithRetry_FailFastOnNonTransient(t *testing.T) {
	fb := &fakeBroker{
		errPermanent: errors.New("validation failed: max debit too low"),
	}
	cfg := Config{
		MaxRetries:     5, // even with higher retries, should not retry on permanent errors
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
		Timeout:        200 * time.Millisecond,
	}
	c, _ := makeClient(t, fb, cfg)

	ctx := context.Background()
	pos := newTestPosition()
	_, err := c.ClosePositionWithRetry(ctx, pos, 0.01)
	if err == nil {
		t.Fatalf("expected error on non-transient failure")
	}
	if atomic.LoadInt32(&fb.callCount) != 1 {
		t.Fatalf("expected only 1 attempt on non-transient error, got %d", fb.callCount)
	}
	if !strings.Contains(err.Error(), "failed to close position") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClosePositionWithRetry_NilPosition(t *testing.T) {
	fb := &fakeBroker{}
	cfg := Config{
		MaxRetries:     1,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
		Timeout:        100 * time.Millisecond,
	}
	c, buf := makeClient(t, fb, cfg)

	ctx := context.Background()
	_, err := c.ClosePositionWithRetry(ctx, nil, 1.0)
	if err == nil {
		t.Fatalf("expected error when position is nil")
	}
	if got := buf.String(); !strings.Contains(got, "nil position") {
		t.Fatalf("expected log mentioning nil position, got: %s", got)
	}
}

func TestClosePositionWithRetry_ContextCanceled(t *testing.T) {
	fb := &fakeBroker{
		// even if broker would succeed, cancellation should preempt
	}
	cfg := Config{
		MaxRetries:     2,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
		Timeout:        1 * time.Second,
	}
	c, _ := makeClient(t, fb, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before call
	pos := newTestPosition()

	_, err := c.ClosePositionWithRetry(ctx, pos, 1.0)
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "operation canceled") {
		t.Fatalf("expected 'operation canceled' in error, got: %v", err)
	}
	// No broker calls should have been made if we checked ctx.Err() early
	if atomic.LoadInt32(&fb.callCount) != 0 {
		t.Fatalf("expected 0 broker calls, got %d", fb.callCount)
	}
}

func TestClosePositionWithRetry_TimeoutDuringBackoff(t *testing.T) {
	// Force transient errors and a short timeout so that we hit the "timed out during backoff" branch.
	fb := &fakeBroker{
		errTransient: errors.New("connection reset"),
	}
	cfg := Config{
		MaxRetries:     10,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
		Timeout:        2 * time.Millisecond, // shorter than backoff
	}
	c, _ := makeClient(t, fb, cfg)

	ctx := context.Background()
	pos := newTestPosition()

	_, err := c.ClosePositionWithRetry(ctx, pos, 1.0)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "timed out") {
		t.Fatalf("expected timeout-related error, got: %v", err)
	}
}

func TestClosePositionWithRetry_TimeoutBeforeCallLoop(t *testing.T) {
	// Ensure we can hit the "timed out after <timeout>" branch by setting an already-expired timeout.
	fb := &fakeBroker{
		errTransient: errors.New("timeout"),
	}
	cfg := Config{
		MaxRetries:     1,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     1 * time.Millisecond,
		Timeout:        1 * time.Nanosecond,
	}
	c, _ := makeClient(t, fb, cfg)

	// Give the timeout a chance to expire
	time.Sleep(2 * time.Nanosecond)

	ctx := context.Background()
	pos := newTestPosition()

	_, err := c.ClosePositionWithRetry(ctx, pos, 1.0)
	if err == nil {
		t.Fatalf("expected timeout error before making broker call")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected 'timed out' in error, got: %v", err)
	}
}