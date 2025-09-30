package main

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
)

// mockBrokerForReconciliation provides a simple broker mock for reconciliation tests
type mockBrokerForReconciliation struct {
	positions []broker.PositionItem
}

// Implement all required Broker interface methods
func (m *mockBrokerForReconciliation) GetAccountBalance() (float64, error) {
	return 100000.0, nil
}

func (m *mockBrokerForReconciliation) GetAccountBalanceCtx(ctx context.Context) (float64, error) {
	return 100000.0, nil
}

func (m *mockBrokerForReconciliation) GetOptionBuyingPower() (float64, error) {
	return 50000.0, nil
}

func (m *mockBrokerForReconciliation) GetOptionBuyingPowerCtx(ctx context.Context) (float64, error) {
	return 50000.0, nil
}

func (m *mockBrokerForReconciliation) GetPositions() ([]broker.PositionItem, error) {
	return m.positions, nil
}

func (m *mockBrokerForReconciliation) GetPositionsCtx(ctx context.Context) ([]broker.PositionItem, error) {
	return m.positions, nil
}

func (m *mockBrokerForReconciliation) GetQuote(symbol string) (*broker.QuoteItem, error) {
	return &broker.QuoteItem{Last: 500.0}, nil
}

func (m *mockBrokerForReconciliation) GetExpirations(symbol string) ([]string, error) {
	return nil, nil
}

func (m *mockBrokerForReconciliation) GetExpirationsCtx(ctx context.Context, symbol string) ([]string, error) {
	return nil, nil
}

func (m *mockBrokerForReconciliation) GetOptionChain(symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	return nil, nil
}

func (m *mockBrokerForReconciliation) GetOptionChainCtx(ctx context.Context, symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	return nil, nil
}

func (m *mockBrokerForReconciliation) GetMarketClock(delayed bool) (*broker.MarketClockResponse, error) {
	return &broker.MarketClockResponse{}, nil
}

func (m *mockBrokerForReconciliation) GetMarketCalendar(month, year int) (*broker.MarketCalendarResponse, error) {
	return &broker.MarketCalendarResponse{}, nil
}

func (m *mockBrokerForReconciliation) GetMarketCalendarCtx(ctx context.Context, month, year int) (*broker.MarketCalendarResponse, error) {
	return &broker.MarketCalendarResponse{}, nil
}

func (m *mockBrokerForReconciliation) IsTradingDay(delayed bool) (bool, error) {
	return true, nil
}

func (m *mockBrokerForReconciliation) GetTickSize(symbol string) (float64, error) {
	return 0.05, nil
}

func (m *mockBrokerForReconciliation) GetHistoricalData(symbol string, interval string, startDate, endDate time.Time) ([]broker.HistoricalDataPoint, error) {
	return nil, nil
}

func (m *mockBrokerForReconciliation) PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string,
	quantity int, limitPrice float64, preview bool, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string,
	quantity int, credit, profitTarget float64, preview bool, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) GetOrderStatus(orderID int) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) GetOrderStatusCtx(ctx context.Context, orderID int) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) CancelOrder(orderID int) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) CancelOrderCtx(ctx context.Context, orderID int) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) GetOrders() (*broker.OrdersResponse, error) {
	return &broker.OrdersResponse{}, nil
}

func (m *mockBrokerForReconciliation) GetOrdersCtx(ctx context.Context) (*broker.OrdersResponse, error) {
	return &broker.OrdersResponse{}, nil
}

func (m *mockBrokerForReconciliation) CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string,
	quantity int, maxDebit float64, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) CloseStranglePositionCtx(ctx context.Context, symbol string, putStrike, callStrike float64, expiration string,
	quantity int, maxDebit float64, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) PlaceBuyToCloseOrder(optionSymbol string, quantity int,
	maxPrice float64, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) PlaceSellToCloseOrder(optionSymbol string, quantity int,
	maxPrice float64, duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) PlaceBuyToCloseMarketOrder(optionSymbol string, quantity int,
	duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) PlaceBuyToCloseMarketOrderCtx(ctx context.Context, optionSymbol string, quantity int,
	duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) PlaceSellToCloseMarketOrder(optionSymbol string, quantity int,
	duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

func (m *mockBrokerForReconciliation) PlaceSellToCloseMarketOrderCtx(ctx context.Context, optionSymbol string, quantity int,
	duration string, tag string) (*broker.OrderResponse, error) {
	return &broker.OrderResponse{}, nil
}

// TestCleanupPhantomPositions tests automatic removal of local-only positions
func TestCleanupPhantomPositions(t *testing.T) {
	// Create temporary storage
	mockStorage, err := storage.NewJSONStorage("test_phantom_positions.json")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer os.Remove("test_phantom_positions.json")

	// Create bot with mock broker and storage
	bot := &Bot{
		broker:  &mockBrokerForReconciliation{positions: []broker.PositionItem{}},
		storage: mockStorage,
		config:  &config.Config{},
		logger:  log.New(os.Stdout, "[TEST] ", log.LstdFlags),
	}

	// Add a phantom position (exists locally but not in broker)
	phantomPos := models.NewPosition(
		"phantom-123",
		"SPY",
		600.0,
		650.0,
		time.Now().AddDate(0, 0, 45),
		1,
	)
	phantomPos.State = models.StateOpen
	phantomPos.CreditReceived = 2.50
	phantomPos.EntryDate = time.Now()

	if err := mockStorage.AddPosition(phantomPos); err != nil {
		t.Fatalf("Failed to add phantom position: %v", err)
	}

	// Verify phantom position exists
	positions := mockStorage.GetCurrentPositions()
	if len(positions) != 1 {
		t.Fatalf("Expected 1 position, got %d", len(positions))
	}

	// Run cleanup
	if err := bot.cleanupPhantomPositions([]string{"phantom-123"}); err != nil {
		t.Fatalf("cleanupPhantomPositions failed: %v", err)
	}

	// Verify phantom position was removed
	positions = mockStorage.GetCurrentPositions()
	if len(positions) != 0 {
		t.Errorf("Expected 0 positions after cleanup, got %d", len(positions))
	}
}

// TestRecoverUntrackedPositions tests automatic recovery of broker-only positions
func TestRecoverUntrackedPositions(t *testing.T) {
	// Create temporary storage
	mockStorage, err := storage.NewJSONStorage("test_recover_positions.json")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer os.Remove("test_recover_positions.json")

	// Create broker positions (call + put strangle)
	expiration := time.Now().AddDate(0, 0, 45)
	expirationStr := expiration.Format("060102")
	brokerPositions := []broker.PositionItem{
		{
			Symbol:     "SPY" + expirationStr + "C00650000",
			Quantity:   -1,
			CostBasis:  -125.0, // Negative = we received credit
		},
		{
			Symbol:     "SPY" + expirationStr + "P00600000",
			Quantity:   -1,
			CostBasis:  -125.0,
		},
	}

	// Create bot with mock broker and storage
	bot := &Bot{
		broker:  &mockBrokerForReconciliation{positions: brokerPositions},
		storage: mockStorage,
		config:  &config.Config{},
		logger:  log.New(os.Stdout, "[TEST] ", log.LstdFlags),
	}

	// Verify no positions exist initially
	positions := mockStorage.GetCurrentPositions()
	if len(positions) != 0 {
		t.Fatalf("Expected 0 initial positions, got %d", len(positions))
	}

	// Run recovery
	untrackedSymbols := []string{
		"SPY" + expirationStr + "C00650000",
		"SPY" + expirationStr + "P00600000",
	}
	if err := bot.recoverUntrackedPositions(context.Background(), brokerPositions, untrackedSymbols); err != nil {
		t.Fatalf("recoverUntrackedPositions failed: %v", err)
	}

	// Verify position was recovered
	positions = mockStorage.GetCurrentPositions()
	if len(positions) != 1 {
		t.Fatalf("Expected 1 recovered position, got %d", len(positions))
	}

	pos := positions[0]
	if pos.Symbol != "SPY" {
		t.Errorf("Expected symbol SPY, got %s", pos.Symbol)
	}
	if pos.CallStrike != 650.0 {
		t.Errorf("Expected call strike 650.0, got %.2f", pos.CallStrike)
	}
	if pos.PutStrike != 600.0 {
		t.Errorf("Expected put strike 600.0, got %.2f", pos.PutStrike)
	}
	if pos.CreditReceived != 250.0 {
		t.Errorf("Expected credit received 250.0, got %.2f", pos.CreditReceived)
	}
	if pos.State != models.StateOpen {
		t.Errorf("Expected state Open, got %s", pos.State)
	}
}

// TestPerformStartupReconciliation tests the full reconciliation flow
func TestPerformStartupReconciliation(t *testing.T) {
	// Create temporary storage
	mockStorage, err := storage.NewJSONStorage("test_reconciliation.json")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer os.Remove("test_reconciliation.json")

	// Create broker positions
	expiration := time.Now().AddDate(0, 0, 45)
	expirationStr := expiration.Format("060102")
	brokerPositions := []broker.PositionItem{
		{
			Symbol:     "SPY" + expirationStr + "C00650000",
			Quantity:   -1,
			CostBasis:  -125.0,
		},
		{
			Symbol:     "SPY" + expirationStr + "P00600000",
			Quantity:   -1,
			CostBasis:  -125.0,
		},
	}

	// Create bot
	bot := &Bot{
		broker:  &mockBrokerForReconciliation{positions: brokerPositions},
		storage: mockStorage,
		config:  &config.Config{},
		logger:  log.New(os.Stdout, "[TEST] ", log.LstdFlags),
	}

	// Add a phantom position
	phantomPos := models.NewPosition(
		"phantom-456",
		"SPY",
		580.0,
		630.0,
		time.Now().AddDate(0, 0, 30),
		1,
	)
	phantomPos.State = models.StateOpen
	phantomPos.CreditReceived = 2.00
	phantomPos.EntryDate = time.Now()

	if err := mockStorage.AddPosition(phantomPos); err != nil {
		t.Fatalf("Failed to add phantom position: %v", err)
	}

	// Run reconciliation
	ctx := context.Background()
	if err := bot.performStartupReconciliation(ctx); err != nil {
		t.Fatalf("performStartupReconciliation failed: %v", err)
	}

	// Verify results:
	// 1. Phantom position should be removed
	// 2. Broker position should be recovered
	positions := mockStorage.GetCurrentPositions()
	if len(positions) != 1 {
		t.Fatalf("Expected 1 position after reconciliation, got %d", len(positions))
	}

	pos := positions[0]
	if pos.CallStrike != 650.0 || pos.PutStrike != 600.0 {
		t.Errorf("Expected recovered broker position with strikes 650/600, got %.2f/%.2f",
			pos.CallStrike, pos.PutStrike)
	}
}

// TestExtractOptionType tests option type extraction from symbols
func TestExtractOptionType(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"SPY251107C00650000", "call"},
		{"SPY251107P00600000", "put"},
		{"INVALID", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		result := extractOptionType(tt.symbol)
		if result != tt.expected {
			t.Errorf("extractOptionType(%s) = %s, expected %s", tt.symbol, result, tt.expected)
		}
	}
}

// TestExtractStrike tests strike price extraction from symbols
func TestExtractStrike(t *testing.T) {
	tests := []struct {
		symbol   string
		expected float64
	}{
		{"SPY251107C00650000", 650.0},
		{"SPY251107P00600000", 600.0},
		{"SPY251107C00123456", 123.456},
		{"INVALID", 0.0},
		{"", 0.0},
	}

	for _, tt := range tests {
		result := extractStrike(tt.symbol)
		if result != tt.expected {
			t.Errorf("extractStrike(%s) = %.3f, expected %.3f", tt.symbol, result, tt.expected)
		}
	}
}

// TestIncompleteStrangleRecovery tests that incomplete strangles are not recovered
func TestIncompleteStrangleRecovery(t *testing.T) {
	mockStorage, err := storage.NewJSONStorage("test_incomplete.json")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer os.Remove("test_incomplete.json")

	// Create only one leg of a strangle
	expiration := time.Now().AddDate(0, 0, 45)
	expirationStr := expiration.Format("060102")
	brokerPositions := []broker.PositionItem{
		{
			Symbol:     "SPY" + expirationStr + "C00650000",
			Quantity:   -1,
			CostBasis:  -125.0,
		},
		// Missing put leg
	}

	bot := &Bot{
		broker:  &mockBrokerForReconciliation{positions: brokerPositions},
		storage: mockStorage,
		config:  &config.Config{},
		logger:  log.New(os.Stdout, "[TEST] ", log.LstdFlags),
	}

	// Run recovery
	untrackedSymbols := []string{"SPY" + expirationStr + "C00650000"}
	if err := bot.recoverUntrackedPositions(context.Background(), brokerPositions, untrackedSymbols); err != nil {
		t.Fatalf("recoverUntrackedPositions failed: %v", err)
	}

	// Verify no position was recovered (incomplete strangle)
	positions := mockStorage.GetCurrentPositions()
	if len(positions) != 0 {
		t.Errorf("Expected 0 positions for incomplete strangle, got %d", len(positions))
	}
}