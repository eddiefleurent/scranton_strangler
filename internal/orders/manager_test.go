package orders

import (
	"context"
	"errors"
	"fmt"
	"math"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
)

// mockBrokerForOrders implements broker.Broker for testing
type mockBrokerForOrders struct {
	orderStatus *broker.OrderResponse
	orderError  error
	callCount   int
}

func (m *mockBrokerForOrders) GetAccountBalance() (float64, error) {
	return 10000.0, nil
}

func (m *mockBrokerForOrders) GetAccountBalanceCtx(ctx context.Context) (float64, error) {
	return 10000.0, nil
}

func (m *mockBrokerForOrders) GetOptionBuyingPower() (float64, error) {
	return 5000.0, nil
}

func (m *mockBrokerForOrders) GetOptionBuyingPowerCtx(ctx context.Context) (float64, error) {
	return 5000.0, nil
}

func (m *mockBrokerForOrders) GetPositions() ([]broker.PositionItem, error) {
	return m.GetPositionsCtx(context.Background())
}

func (m *mockBrokerForOrders) GetPositionsCtx(ctx context.Context) ([]broker.PositionItem, error) {
	return []broker.PositionItem{}, nil
}

func (m *mockBrokerForOrders) GetQuote(symbol string) (*broker.QuoteItem, error) {
	return &broker.QuoteItem{Symbol: symbol, Last: 100.0}, nil
}

func (m *mockBrokerForOrders) GetExpirations(symbol string) ([]string, error) {
	return []string{"2024-12-20"}, nil
}

func (m *mockBrokerForOrders) GetExpirationsCtx(ctx context.Context, symbol string) ([]string, error) {
	return m.GetExpirations(symbol)
}

func (m *mockBrokerForOrders) GetOptionChain(symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	return []broker.Option{}, nil
}

func (m *mockBrokerForOrders) GetOptionChainCtx(ctx context.Context, symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	return []broker.Option{}, nil
}

func (m *mockBrokerForOrders) PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string, quantity int, limitPrice float64, preview bool, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForOrders) PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string, quantity int, credit, profitTarget float64, preview bool, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForOrders) GetOrderStatus(orderID int) (*broker.OrderResponse, error) {
	m.callCount++
	return m.orderStatus, m.orderError
}

func (m *mockBrokerForOrders) GetOrderStatusCtx(ctx context.Context, orderID int) (*broker.OrderResponse, error) {
	m.callCount++
	return m.orderStatus, m.orderError
}

func (m *mockBrokerForOrders) CancelOrder(orderID int) (*broker.OrderResponse, error) {
	m.callCount++
	resp := &broker.OrderResponse{}
	resp.Order.ID = orderID
	resp.Order.Status = "canceled"
	return resp, nil
}

func (m *mockBrokerForOrders) CancelOrderCtx(ctx context.Context, orderID int) (*broker.OrderResponse, error) {
	return m.CancelOrder(orderID)
}

func (m *mockBrokerForOrders) GetOrders() (*broker.OrdersResponse, error) {
	m.callCount++
	return &broker.OrdersResponse{}, nil
}

func (m *mockBrokerForOrders) GetOrdersCtx(ctx context.Context) (*broker.OrdersResponse, error) {
	return m.GetOrders()
}

func (m *mockBrokerForOrders) CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string, quantity int, maxDebit float64, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForOrders) CloseStranglePositionCtx(ctx context.Context, symbol string, putStrike, callStrike float64, expiration string, quantity int, maxDebit float64, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForOrders) PlaceBuyToCloseOrder(optionSymbol string, quantity int, maxPrice float64, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForOrders) PlaceSellToCloseOrder(optionSymbol string, quantity int, maxPrice float64, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForOrders) PlaceBuyToCloseMarketOrder(optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForOrders) PlaceSellToCloseMarketOrder(optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForOrders) PlaceBuyToCloseMarketOrderCtx(ctx context.Context, optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	return m.PlaceBuyToCloseMarketOrder(optionSymbol, quantity, duration, tag)
}

func (m *mockBrokerForOrders) PlaceSellToCloseMarketOrderCtx(ctx context.Context, optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	return m.PlaceSellToCloseMarketOrder(optionSymbol, quantity, duration, tag)
}

func (m *mockBrokerForOrders) GetMarketClock(delayed bool) (*broker.MarketClockResponse, error) {
	return &broker.MarketClockResponse{}, nil
}

func (m *mockBrokerForOrders) GetMarketCalendar(month, year int) (*broker.MarketCalendarResponse, error) {
	return &broker.MarketCalendarResponse{}, nil
}

func (m *mockBrokerForOrders) GetMarketCalendarCtx(ctx context.Context, month, year int) (*broker.MarketCalendarResponse, error) {
	return &broker.MarketCalendarResponse{}, nil
}

func (m *mockBrokerForOrders) IsTradingDay(delayed bool) (bool, error) {
	return true, nil
}

func (m *mockBrokerForOrders) GetTickSize(symbol string) (float64, error) {
	return 0.01, nil
}

func (m *mockBrokerForOrders) GetHistoricalData(symbol string, interval string, startDate, endDate time.Time) ([]broker.HistoricalDataPoint, error) {
	return []broker.HistoricalDataPoint{}, nil
}

func TestNewManager_DefaultConfig(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockBroker := &mockBrokerForOrders{}
	mockStorage := storage.NewMockStorage()

	// Test with nil config (should use defaults)
	m := NewManager(mockBroker, mockStorage, logger, nil)

	if m.broker != mockBroker {
		t.Error("broker not set correctly")
	}
	if m.storage != mockStorage {
		t.Error("storage not set correctly")
	}
	if m.logger == nil {
		t.Error("logger should not be nil")
	}
	if m.config.PollInterval != DefaultConfig.PollInterval {
		t.Errorf("expected PollInterval %v, got %v", DefaultConfig.PollInterval, m.config.PollInterval)
	}
	if m.config.Timeout != DefaultConfig.Timeout {
		t.Errorf("expected Timeout %v, got %v", DefaultConfig.Timeout, m.config.Timeout)
	}
	if m.config.CallTimeout != DefaultConfig.CallTimeout {
		t.Errorf("expected CallTimeout %v, got %v", DefaultConfig.CallTimeout, m.config.CallTimeout)
	}
}

func TestNewManager_CustomConfig(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockBroker := &mockBrokerForOrders{}
	mockStorage := storage.NewMockStorage()

	customConfig := Config{
		PollInterval: 10 * time.Second,
		Timeout:      10 * time.Minute,
		CallTimeout:  10 * time.Second,
	}

	m := NewManager(mockBroker, mockStorage, logger, nil, customConfig)

	if m.config.PollInterval != customConfig.PollInterval {
		t.Errorf("expected PollInterval %v, got %v", customConfig.PollInterval, m.config.PollInterval)
	}
	if m.config.Timeout != customConfig.Timeout {
		t.Errorf("expected Timeout %v, got %v", customConfig.Timeout, m.config.Timeout)
	}
	if m.config.CallTimeout != customConfig.CallTimeout {
		t.Errorf("expected CallTimeout %v, got %v", customConfig.CallTimeout, m.config.CallTimeout)
	}
}

func TestNewManager_ConfigValidation(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockBroker := &mockBrokerForOrders{}
	mockStorage := storage.NewMockStorage()

	// Test with invalid config values (should be clamped to defaults)
	invalidConfig := Config{
		PollInterval: 0,
		Timeout:      0,
		CallTimeout:  0,
	}

	m := NewManager(mockBroker, mockStorage, logger, nil, invalidConfig)

	if m.config.PollInterval != DefaultConfig.PollInterval {
		t.Errorf("expected PollInterval to be clamped to %v, got %v", DefaultConfig.PollInterval, m.config.PollInterval)
	}
	if m.config.Timeout != DefaultConfig.Timeout {
		t.Errorf("expected Timeout to be clamped to %v, got %v", DefaultConfig.Timeout, m.config.Timeout)
	}
	if m.config.CallTimeout != DefaultConfig.CallTimeout {
		t.Errorf("expected CallTimeout to be clamped to %v, got %v", DefaultConfig.CallTimeout, m.config.CallTimeout)
	}
}

func TestNewManager_NilLogger(t *testing.T) {
	mockBroker := &mockBrokerForOrders{}
	mockStorage := storage.NewMockStorage()

	// Test with nil logger (should create a default logger)
	m := NewManager(mockBroker, mockStorage, nil, nil)

	if m.logger == nil {
		t.Error("logger should not be nil even when passed nil")
	}
}

func TestManager_IsOrderTerminal(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockStorage := storage.NewMockStorage()
	ctx := context.Background()

	tests := []struct {
		name         string
		orderStatus  string
		orderID      int
		expectError  bool
		expectResult bool
	}{
		{"filled order", "filled", 123, false, true},
		{"canceled order", "canceled", 123, false, true},
		{"cancelled order", "cancelled", 123, false, true},
		{"rejected order", "rejected", 123, false, true},
		{"expired order", "expired", 123, false, true},
		{"pending order", "pending", 123, false, false},
		{"open order", "open", 123, false, false},
		{"partial order", "partial", 123, false, false},
		{"unknown status", "unknown", 123, false, false},
		{"nil response", "", 123, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var orderResp *broker.OrderResponse
			if tt.orderStatus != "" {
				orderResp = &broker.OrderResponse{
					Order: struct {
						CreateDate        string  `json:"create_date"`
						Type              string  `json:"type"`
						Symbol            string  `json:"symbol"`
						Side              string  `json:"side"`
						Class             string  `json:"class"`
						Status            string  `json:"status"`
						Duration          string  `json:"duration"`
						TransactionDate   string  `json:"transaction_date"`
						AvgFillPrice      float64 `json:"avg_fill_price"`
						ExecQuantity      float64 `json:"exec_quantity"`
						LastFillPrice     float64 `json:"last_fill_price"`
						LastFillQuantity  float64 `json:"last_fill_quantity"`
						RemainingQuantity float64 `json:"remaining_quantity"`
						ID                int     `json:"id"`
						Price             float64 `json:"price"`
						Quantity          float64 `json:"quantity"`
					}{
						ID:     tt.orderID,
						Status: tt.orderStatus,
					},
				}
			}

			mockBroker := &mockBrokerForOrders{
				orderStatus: orderResp,
				orderError:  nil,
			}

			m := NewManager(mockBroker, mockStorage, logger, nil)

			result, err := m.IsOrderTerminal(ctx, tt.orderID)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expectResult {
					t.Errorf("expected result %v, got %v", tt.expectResult, result)
				}
			}
		})
	}
}

func TestManager_IsOrderTerminal_BrokerError(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockStorage := storage.NewMockStorage()
	ctx := context.Background()

	mockBroker := &mockBrokerForOrders{
		orderStatus: nil,
		orderError:  errors.New("broker error"),
	}

	m := NewManager(mockBroker, mockStorage, logger, nil)

	_, err := m.IsOrderTerminal(ctx, 123)
	if err == nil {
		t.Error("expected error from broker, got none")
	}
	if !strings.Contains(err.Error(), "failed to get order status") {
		t.Errorf("expected error to contain 'failed to get order status', got: %v", err)
	}
}

func TestManager_PollOrderStatus_OrderFilled(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockStorage := storage.NewMockStorage()

	// Create a position in StateSubmitted (entry order)
	position := models.NewPosition("test-pos", "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)
	err := position.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to set up test position: %v", err)
	}

	if err := mockStorage.AddPosition(position); err != nil {
		t.Fatalf("Failed to set up test position in storage: %v", err)
	}

	// Mock broker that returns "filled" status immediately
	orderResp := &broker.OrderResponse{
		Order: struct {
			CreateDate        string  `json:"create_date"`
			Type              string  `json:"type"`
			Symbol            string  `json:"symbol"`
			Side              string  `json:"side"`
			Class             string  `json:"class"`
			Status            string  `json:"status"`
			Duration          string  `json:"duration"`
			TransactionDate   string  `json:"transaction_date"`
			AvgFillPrice      float64 `json:"avg_fill_price"`
			ExecQuantity      float64 `json:"exec_quantity"`
			LastFillPrice     float64 `json:"last_fill_price"`
			LastFillQuantity  float64 `json:"last_fill_quantity"`
			RemainingQuantity float64 `json:"remaining_quantity"`
			ID                int     `json:"id"`
			Price             float64 `json:"price"`
			Quantity          float64 `json:"quantity"`
		}{
			ID:     123,
			Status: "filled",
		},
	}

	mockBroker := &mockBrokerForOrders{
		orderStatus: orderResp,
		orderError:  nil,
	}

	m := NewManager(mockBroker, mockStorage, logger, nil, Config{
		PollInterval: 1 * time.Millisecond, // Very fast polling for test
		Timeout:      1 * time.Second,
		CallTimeout:  100 * time.Millisecond,
	})

	// Start polling in a goroutine
	done := make(chan bool)
	go func() {
		m.PollOrderStatus("test-pos", 123, true) // true = entry order
		done <- true
	}()

	// Wait for polling to complete
	select {
	case <-done:
		// Success - polling completed
	case <-time.After(2 * time.Second):
		t.Fatal("PollOrderStatus did not complete within timeout")
	}

	// Verify position was transitioned to StateOpen
	updatedPosition, found := mockStorage.GetPositionByID("test-pos")
	if !found {
		t.Fatal("Expected to find updated position")
	}
	if updatedPosition.GetCurrentState() != models.StateOpen {
		t.Errorf("Expected position state to be %s, got %s", models.StateOpen, updatedPosition.GetCurrentState())
	}
}

func TestManager_PollOrderStatus_OrderCanceled(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockStorage := storage.NewMockStorage()

	// Create a position in StateSubmitted (entry order)
	position := models.NewPosition("test-pos", "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)
	err := position.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to set up test position: %v", err)
	}

	if err := mockStorage.AddPosition(position); err != nil {
		t.Fatalf("Failed to set up test position in storage: %v", err)
	}

	// Mock broker that returns "canceled" status
	orderResp := &broker.OrderResponse{
		Order: struct {
			CreateDate        string  `json:"create_date"`
			Type              string  `json:"type"`
			Symbol            string  `json:"symbol"`
			Side              string  `json:"side"`
			Class             string  `json:"class"`
			Status            string  `json:"status"`
			Duration          string  `json:"duration"`
			TransactionDate   string  `json:"transaction_date"`
			AvgFillPrice      float64 `json:"avg_fill_price"`
			ExecQuantity      float64 `json:"exec_quantity"`
			LastFillPrice     float64 `json:"last_fill_price"`
			LastFillQuantity  float64 `json:"last_fill_quantity"`
			RemainingQuantity float64 `json:"remaining_quantity"`
			ID                int     `json:"id"`
			Price             float64 `json:"price"`
			Quantity          float64 `json:"quantity"`
		}{
			ID:     123,
			Status: "canceled",
		},
	}

	mockBroker := &mockBrokerForOrders{
		orderStatus: orderResp,
		orderError:  nil,
	}

	m := NewManager(mockBroker, mockStorage, logger, nil, Config{
		PollInterval: 1 * time.Millisecond,
		Timeout:      1 * time.Second,
		CallTimeout:  100 * time.Millisecond,
	})

	// Start polling
	done := make(chan bool)
	go func() {
		m.PollOrderStatus("test-pos", 123, true)
		done <- true
	}()

	// Wait for polling to complete
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("PollOrderStatus did not complete within timeout")
	}

	// Verify position was transitioned to StateError
	updatedPosition, found := mockStorage.GetPositionByID("test-pos")
	if !found {
		t.Fatal("Expected to find updated position")
	}
	if updatedPosition.GetCurrentState() != models.StateError {
		t.Errorf("Expected position state to be %s, got %s", models.StateError, updatedPosition.GetCurrentState())
	}
}

func TestManager_PollOrderStatus_Timeout(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockStorage := storage.NewMockStorage()

	// Create a position in StateSubmitted
	position := models.NewPosition("test-pos", "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)
	err := position.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to set up test position: %v", err)
	}

	if err := mockStorage.AddPosition(position); err != nil {
		t.Fatalf("Failed to set up test position in storage: %v", err)
	}

	// Mock broker that always returns pending status
	orderResp := &broker.OrderResponse{
		Order: struct {
			CreateDate        string  `json:"create_date"`
			Type              string  `json:"type"`
			Symbol            string  `json:"symbol"`
			Side              string  `json:"side"`
			Class             string  `json:"class"`
			Status            string  `json:"status"`
			Duration          string  `json:"duration"`
			TransactionDate   string  `json:"transaction_date"`
			AvgFillPrice      float64 `json:"avg_fill_price"`
			ExecQuantity      float64 `json:"exec_quantity"`
			LastFillPrice     float64 `json:"last_fill_price"`
			LastFillQuantity  float64 `json:"last_fill_quantity"`
			RemainingQuantity float64 `json:"remaining_quantity"`
			ID                int     `json:"id"`
			Price             float64 `json:"price"`
			Quantity          float64 `json:"quantity"`
		}{
			ID:     123,
			Status: "pending",
		},
	}

	mockBroker := &mockBrokerForOrders{
		orderStatus: orderResp,
		orderError:  nil,
	}

	m := NewManager(mockBroker, mockStorage, logger, nil, Config{
		PollInterval: 1 * time.Millisecond,
		Timeout:      10 * time.Millisecond, // Very short timeout
		CallTimeout:  5 * time.Millisecond,
	})

	// Start polling
	done := make(chan bool)
	go func() {
		m.PollOrderStatus("test-pos", 123, true)
		done <- true
	}()

	// Wait for polling to complete due to timeout
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("PollOrderStatus did not complete within timeout")
	}

	// Verify position was closed due to timeout
	_, found := mockStorage.GetPositionByID("test-pos")
	if found {
		t.Error("Expected current position to not be found after timeout close")
	}

	// Verify position exists in history
	history := mockStorage.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 position in history, got %d", len(history))
		return
	}

	if history[0].GetCurrentState() != models.StateClosed {
		t.Errorf("Expected historical position to be closed, got %s", history[0].GetCurrentState())
	}
}

func TestManager_TimeoutTransitionReasons(t *testing.T) {
	// Test the timeoutTransitionReason function for different states
	m := &Manager{}

	tests := []struct {
		currentState   models.PositionState
		expectedReason string
		description    string
	}{
		{models.StateAdjusting, "hard_stop", "StateAdjusting should use hard_stop"},
		{models.StateRolling, "force_close", "StateRolling should use force_close"},
		{models.StateFirstDown, "exit_conditions", "StateFirstDown should use exit_conditions"},
		{models.StateSecondDown, "exit_conditions", "StateSecondDown should use exit_conditions"},
		{models.StateThirdDown, "hard_stop", "StateThirdDown should use hard_stop"},
		{models.StateFourthDown, "emergency_exit", "StateFourthDown should use emergency_exit"},
		{models.StateError, "force_close", "StateError should use force_close"},
		{models.StateIdle, "force_close", "Unknown state should default to force_close"},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			result := m.timeoutTransitionReason(test.currentState)
			if result != test.expectedReason {
				t.Errorf("Expected %s for state %s, got %s", test.expectedReason, test.currentState, result)
			}
		})
	}
}

func TestManager_HandleOrderTimeout_EntryOrder(t *testing.T) {
	// Test entry order timeout (StateSubmitted -> StateClosed with "order_timeout")
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockStorage := storage.NewMockStorage()

	// Create a position in StateSubmitted (entry order)
	position := models.NewPosition("test-pos", "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)
	err := position.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to set up test position: %v", err)
	}

	if err := mockStorage.AddPosition(position); err != nil {
		t.Fatalf("Failed to set up test position in storage: %v", err)
	}

	mockBroker := &mockBrokerForOrders{}
	m := NewManager(mockBroker, mockStorage, logger, nil)

	// Simulate entry order timeout
	m.handleOrderTimeout("test-pos")

	// Verify position was closed and moved to history (current position should be nil)
	_, found := mockStorage.GetPositionByID("test-pos")
	if found {
		t.Error("Expected current position to not be found after close")
	}

	// Verify position exists in history and is closed
	history := mockStorage.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 position in history, got %d", len(history))
		return
	}

	closedPosition := history[0]
	if closedPosition.GetCurrentState() != models.StateClosed {
		t.Errorf("Expected position in history to be closed, got %s", closedPosition.GetCurrentState())
	}
}

func TestManager_HandleOrderTimeout_ExitOrderFromAdjusting(t *testing.T) {
	// Test exit order timeout from StateAdjusting -> StateClosed with "hard_stop"
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockStorage := storage.NewMockStorage()

	// Create a position and follow proper state transitions to StateAdjusting
	position := models.NewPosition("test-pos", "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)
	position.EntryOrderID = "123" // Set as if entry order was placed

	// Follow proper state flow: Idle -> Submitted -> Open -> FirstDown -> SecondDown -> Adjusting
	err := position.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		t.Fatalf("Failed to transition to submitted: %v", err)
	}
	err = position.TransitionState(models.StateOpen, "order_filled")
	if err != nil {
		t.Fatalf("Failed to transition to open: %v", err)
	}
	err = position.TransitionState(models.StateFirstDown, "start_management")
	if err != nil {
		t.Fatalf("Failed to transition to first down: %v", err)
	}
	err = position.TransitionState(models.StateSecondDown, "strike_challenged")
	if err != nil {
		t.Fatalf("Failed to transition to second down: %v", err)
	}
	err = position.TransitionState(models.StateAdjusting, "roll_untested")
	if err != nil {
		t.Fatalf("Failed to transition to adjusting: %v", err)
	}

	// Set up exit order scenario
	position.ExitOrderID = "456" // Exit order is active
	position.ExitReason = "stop_loss"

	if err := mockStorage.AddPosition(position); err != nil {
		t.Fatalf("Failed to set up test position in storage: %v", err)
	}

	mockBroker := &mockBrokerForOrders{}
	m := NewManager(mockBroker, mockStorage, logger, nil)

	// Simulate exit order timeout
	m.handleOrderTimeout("test-pos")

	// Verify position was closed and moved to history (current position should be nil)
	_, found := mockStorage.GetPositionByID("test-pos")
	if found {
		t.Error("Expected current position to not be found after close")
	}

	// Verify position exists in history and is closed
	history := mockStorage.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 position in history, got %d", len(history))
		return
	}

	closedPosition := history[0]
	if closedPosition.GetCurrentState() != models.StateClosed {
		t.Errorf("Expected position in history to be closed, got %s", closedPosition.GetCurrentState())
	}
}

func TestManager_ExitConditionFromReason(t *testing.T) {
	// Test the exitConditionFromReason mapping
	m := &Manager{}

	tests := []struct {
		exitReason string
		expected   string
	}{
		{"profit_target", "exit_conditions"},
		{"time", "exit_conditions"},
		{"manual", "exit_conditions"},
		{"escalate", "emergency_exit"},
		{"stop_loss", "hard_stop"},
		{"error", "hard_stop"},
		{"unknown", "exit_conditions"}, // default
	}

	for _, test := range tests {
		t.Run(test.exitReason, func(t *testing.T) {
			result := m.exitConditionFromReason(test.exitReason)
			if result != test.expected {
				t.Errorf("Expected %s for exit reason %s, got %s", test.expected, test.exitReason, result)
			}
		})
	}
}

// TestManager_IsOrderCompletelyFilled tests the partial fill detection logic
func TestManager_HandleOrderFill_CreditVsDebitFills(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)

	tests := []struct {
		name               string
		orderType          string
		avgFillPrice       float64
		execQuantity       float64
		expectedCredit     float64
		expectedQuantity   int
		expectCreditSet    bool
		description        string
	}{
		{
			name:            "credit_order_positive_fill_price",
			orderType:       "credit",
			avgFillPrice:    2.50,
			execQuantity:    2.0,
			expectedCredit:  2.50,
			expectedQuantity: 2,
			expectCreditSet: true,
			description:     "Credit order with positive fill price should set CreditReceived to absolute value",
		},
		{
			name:            "credit_order_negative_fill_price",
			orderType:       "credit",
			avgFillPrice:    -2.50,
			execQuantity:    2.0,
			expectedCredit:  2.50,
			expectedQuantity: 2,
			expectCreditSet: true,
			description:     "Credit order with negative fill price should set CreditReceived to absolute value",
		},
		{
			name:            "debit_order_positive_fill_price",
			orderType:       "debit",
			avgFillPrice:    3.00,
			execQuantity:    1.5,
			expectedCredit:  0.0,
			expectedQuantity: 2, // Rounded from 1.5
			expectCreditSet: false,
			description:     "Debit order should not set CreditReceived regardless of fill price",
		},
		{
			name:            "debit_order_negative_fill_price",
			orderType:       "debit",
			avgFillPrice:    -1.25,
			execQuantity:    3.7,
			expectedCredit:  0.0,
			expectedQuantity: 4, // Rounded from 3.7
			expectCreditSet: false,
			description:     "Debit order with negative fill should not set CreditReceived",
		},
		{
			name:            "multileg_order_type",
			orderType:       "multileg",
			avgFillPrice:    1.75,
			execQuantity:    1.0,
			expectedCredit:  0.0,
			expectedQuantity: 1,
			expectCreditSet: false,
			description:     "Non-credit order types should not set CreditReceived",
		},
		{
			name:            "market_order_type",
			orderType:       "market",
			avgFillPrice:    2.25,
			execQuantity:    2.3,
			expectedCredit:  0.0,
			expectedQuantity: 2, // Rounded from 2.3
			expectCreditSet: false,
			description:     "Market orders should not set CreditReceived",
		},
		{
			name:             "zero_exec_quantity_no_effect",
			orderType:        "credit",
			avgFillPrice:     1.00,
			execQuantity:     0.0,
			expectedCredit:   0.0,
			expectedQuantity: 0, // should remain 0 as StateSubmitted positions have quantity 0
			expectCreditSet:  false,
			description:      "0 exec quantity should not alter position or credit",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh storage for each test
			testStorage := storage.NewMockStorage()
			
			// Create a position in StateSubmitted (entry order) with unique ID
			positionID := fmt.Sprintf("test-pos-%d", i)
			position := models.NewPosition(positionID, "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)
			position.EntryOrderID = "123"
			err := position.TransitionState(models.StateSubmitted, "order_placed")
			if err != nil {
				t.Fatalf("Failed to set up test position: %v", err)
			}

			if err := testStorage.AddPosition(position); err != nil {
				t.Fatalf("Failed to set up test position in storage: %v", err)
			}

			// Mock broker that returns filled order with specific type and fill price
			orderResp := &broker.OrderResponse{
				Order: struct {
					CreateDate        string  `json:"create_date"`
					Type              string  `json:"type"`
					Symbol            string  `json:"symbol"`
					Side              string  `json:"side"`
					Class             string  `json:"class"`
					Status            string  `json:"status"`
					Duration          string  `json:"duration"`
					TransactionDate   string  `json:"transaction_date"`
					AvgFillPrice      float64 `json:"avg_fill_price"`
					ExecQuantity      float64 `json:"exec_quantity"`
					LastFillPrice     float64 `json:"last_fill_price"`
					LastFillQuantity  float64 `json:"last_fill_quantity"`
					RemainingQuantity float64 `json:"remaining_quantity"`
					ID                int     `json:"id"`
					Price             float64 `json:"price"`
					Quantity          float64 `json:"quantity"`
				}{
					ID:           123,
					Status:       "filled",
					Type:         tt.orderType,
					AvgFillPrice: tt.avgFillPrice,
					ExecQuantity: tt.execQuantity,
				},
			}

			mockBroker := &mockBrokerForOrders{
				orderStatus: orderResp,
				orderError:  nil,
			}

			m := NewManager(mockBroker, testStorage, logger, nil, Config{
				PollInterval: 1 * time.Millisecond,
				Timeout:      1 * time.Second,
				CallTimeout:  100 * time.Millisecond,
			})

			// Start polling in a goroutine
			done := make(chan bool)
			go func() {
				m.PollOrderStatus(positionID, 123, true) // true = entry order
				done <- true
			}()

			// Wait for polling to complete
			select {
			case <-done:
				// Success - polling completed
			case <-time.After(2 * time.Second):
				t.Fatal("PollOrderStatus did not complete within timeout")
			}

			// Verify position was transitioned to StateOpen
			updatedPosition, found := testStorage.GetPositionByID(positionID)
			if !found {
				t.Fatal("Expected to find updated position")
			}
			if updatedPosition.GetCurrentState() != models.StateOpen {
				t.Errorf("Expected position state to be %s, got %s", models.StateOpen, updatedPosition.GetCurrentState())
			}

			// Verify quantity is properly rounded
			if updatedPosition.Quantity != tt.expectedQuantity {
				t.Errorf("Expected quantity %d, got %d", tt.expectedQuantity, updatedPosition.Quantity)
			}

			// Verify credit received handling based on order type
			if tt.expectCreditSet {
				if math.Abs(updatedPosition.CreditReceived-tt.expectedCredit) > 1e-6 {
					t.Errorf("Expected CreditReceived %.4f, got %.4f", tt.expectedCredit, updatedPosition.CreditReceived)
				}
			} else {
				if math.Abs(updatedPosition.CreditReceived-0.0) > 1e-6 {
					t.Errorf("Expected CreditReceived to remain 0 for %s order, got %.4f", tt.orderType, updatedPosition.CreditReceived)
				}
			}
		})
	}
}

func TestManager_IsOrderCompletelyFilled(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	mockBroker := &mockBrokerForOrders{}
	mockStorage := storage.NewMockStorage()
	stop := make(chan struct{})
	defer close(stop)
	
	manager := NewManager(mockBroker, mockStorage, logger, stop)

	tests := []struct {
		name           string
		orderResponse  *broker.OrderResponse
		expectedResult bool
		description    string
	}{
		{
			name: "explicitly_filled_status",
			orderResponse: &broker.OrderResponse{
				Order: struct {
					CreateDate        string  `json:"create_date"`
					Type              string  `json:"type"`
					Symbol            string  `json:"symbol"`
					Side              string  `json:"side"`
					Class             string  `json:"class"`
					Status            string  `json:"status"`
					Duration          string  `json:"duration"`
					TransactionDate   string  `json:"transaction_date"`
					AvgFillPrice      float64 `json:"avg_fill_price"`
					ExecQuantity      float64 `json:"exec_quantity"`
					LastFillPrice     float64 `json:"last_fill_price"`
					LastFillQuantity  float64 `json:"last_fill_quantity"`
					RemainingQuantity float64 `json:"remaining_quantity"`
					ID                int     `json:"id"`
					Price             float64 `json:"price"`
					Quantity          float64 `json:"quantity"`
				}{
					Status:            "filled",
					ExecQuantity:      3.0,
					Quantity:          3.0,
					RemainingQuantity: 0.0,
				},
			},
			expectedResult: true,
			description:    "Order with 'filled' status should be considered complete",
		},
		{
			name: "partial_status_but_fully_executed",
			orderResponse: &broker.OrderResponse{
				Order: struct {
					CreateDate        string  `json:"create_date"`
					Type              string  `json:"type"`
					Symbol            string  `json:"symbol"`
					Side              string  `json:"side"`
					Class             string  `json:"class"`
					Status            string  `json:"status"`
					Duration          string  `json:"duration"`
					TransactionDate   string  `json:"transaction_date"`
					AvgFillPrice      float64 `json:"avg_fill_price"`
					ExecQuantity      float64 `json:"exec_quantity"`
					LastFillPrice     float64 `json:"last_fill_price"`
					LastFillQuantity  float64 `json:"last_fill_quantity"`
					RemainingQuantity float64 `json:"remaining_quantity"`
					ID                int     `json:"id"`
					Price             float64 `json:"price"`
					Quantity          float64 `json:"quantity"`
				}{
					Status:            "partial",
					ExecQuantity:      3.0,
					Quantity:          3.0,
					RemainingQuantity: 0.0,
				},
			},
			expectedResult: true,
			description:    "Order with 'partial' status but exec_quantity == quantity should be complete",
		},
		{
			name: "partially_filled_status_with_remaining",
			orderResponse: &broker.OrderResponse{
				Order: struct {
					CreateDate        string  `json:"create_date"`
					Type              string  `json:"type"`
					Symbol            string  `json:"symbol"`
					Side              string  `json:"side"`
					Class             string  `json:"class"`
					Status            string  `json:"status"`
					Duration          string  `json:"duration"`
					TransactionDate   string  `json:"transaction_date"`
					AvgFillPrice      float64 `json:"avg_fill_price"`
					ExecQuantity      float64 `json:"exec_quantity"`
					LastFillPrice     float64 `json:"last_fill_price"`
					LastFillQuantity  float64 `json:"last_fill_quantity"`
					RemainingQuantity float64 `json:"remaining_quantity"`
					ID                int     `json:"id"`
					Price             float64 `json:"price"`
					Quantity          float64 `json:"quantity"`
				}{
					Status:            "partially_filled",
					ExecQuantity:      1.0,
					Quantity:          3.0,
					RemainingQuantity: 2.0,
				},
			},
			expectedResult: false,
			description:    "Truly partial order should not be considered complete",
		},
		{
			name: "zero_remaining_quantity",
			orderResponse: &broker.OrderResponse{
				Order: struct {
					CreateDate        string  `json:"create_date"`
					Type              string  `json:"type"`
					Symbol            string  `json:"symbol"`
					Side              string  `json:"side"`
					Class             string  `json:"class"`
					Status            string  `json:"status"`
					Duration          string  `json:"duration"`
					TransactionDate   string  `json:"transaction_date"`
					AvgFillPrice      float64 `json:"avg_fill_price"`
					ExecQuantity      float64 `json:"exec_quantity"`
					LastFillPrice     float64 `json:"last_fill_price"`
					LastFillQuantity  float64 `json:"last_fill_quantity"`
					RemainingQuantity float64 `json:"remaining_quantity"`
					ID                int     `json:"id"`
					Price             float64 `json:"price"`
					Quantity          float64 `json:"quantity"`
				}{
					Status:            "open",
					ExecQuantity:      2.999999, // slightly under due to precision
					Quantity:          3.0,
					RemainingQuantity: 0.000001, // essentially zero
				},
			},
			expectedResult: true,
			description:    "Order with zero remaining should be complete (handles floating point precision)",
		},
		{
			name: "rejected_order_with_zero_remaining",
			orderResponse: &broker.OrderResponse{
				Order: struct {
					CreateDate        string  `json:"create_date"`
					Type              string  `json:"type"`
					Symbol            string  `json:"symbol"`
					Side              string  `json:"side"`
					Class             string  `json:"class"`
					Status            string  `json:"status"`
					Duration          string  `json:"duration"`
					TransactionDate   string  `json:"transaction_date"`
					AvgFillPrice      float64 `json:"avg_fill_price"`
					ExecQuantity      float64 `json:"exec_quantity"`
					LastFillPrice     float64 `json:"last_fill_price"`
					LastFillQuantity  float64 `json:"last_fill_quantity"`
					RemainingQuantity float64 `json:"remaining_quantity"`
					ID                int     `json:"id"`
					Price             float64 `json:"price"`
					Quantity          float64 `json:"quantity"`
				}{
					Status:            "rejected", // Order was rejected
					ExecQuantity:      0.0,        // Nothing executed
					Quantity:          6.0,        // Requested 6 contracts
					RemainingQuantity: 0.0,        // Nothing remaining (because rejected)
				},
			},
			expectedResult: false,
			description:    "Rejected order with zero executed and zero remaining should NOT be considered complete",
		},
		{
			name: "nil_order_response",
			orderResponse: nil,
			expectedResult: false,
			description:   "Nil order response should return false",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := manager.isOrderCompletelyFilled(test.orderResponse)
			if result != test.expectedResult {
				t.Errorf("%s: Expected %t, got %t", test.description, test.expectedResult, result)
				if test.orderResponse != nil {
					t.Logf("Order details: Status=%s, ExecQty=%.6f, TotalQty=%.6f, Remaining=%.6f",
						test.orderResponse.Order.Status,
						test.orderResponse.Order.ExecQuantity,
						test.orderResponse.Order.Quantity,
						test.orderResponse.Order.RemainingQuantity)
				}
			}
		})
	}
}
