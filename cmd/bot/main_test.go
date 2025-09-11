package main

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/orders"
	"github.com/eddiefleurent/scranton_strangler/internal/retry"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/mock"
)

// MockBroker for testing - implements broker.Broker interface
type MockBroker struct {
	mock.Mock
}

func NewMockBroker() *MockBroker {
	return &MockBroker{}
}

// Implement all broker.Broker interface methods
func (m *MockBroker) GetAccountBalance() (float64, error) {
	args := m.Called()
	return args.Get(0).(float64), args.Error(1)
}

func (m *MockBroker) GetAccountBalanceCtx(ctx context.Context) (float64, error) {
	args := m.Called(ctx)
	return args.Get(0).(float64), args.Error(1)
}

func (m *MockBroker) GetOptionBuyingPower() (float64, error) {
	args := m.Called()
	return args.Get(0).(float64), args.Error(1)
}

func (m *MockBroker) GetOptionBuyingPowerCtx(ctx context.Context) (float64, error) {
	args := m.Called(ctx)
	return args.Get(0).(float64), args.Error(1)
}

func (m *MockBroker) GetPositions() ([]broker.PositionItem, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]broker.PositionItem), args.Error(1)
}

func (m *MockBroker) GetPositionsCtx(ctx context.Context) ([]broker.PositionItem, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]broker.PositionItem), args.Error(1)
}

func (m *MockBroker) GetQuote(symbol string) (*broker.QuoteItem, error) {
	args := m.Called(symbol)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.QuoteItem), args.Error(1)
}

func (m *MockBroker) GetExpirations(symbol string) ([]string, error) {
	args := m.Called(symbol)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockBroker) GetExpirationsCtx(ctx context.Context, symbol string) ([]string, error) {
	args := m.Called(ctx, symbol)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockBroker) GetOptionChain(symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	args := m.Called(symbol, expiration, withGreeks)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]broker.Option), args.Error(1)
}

func (m *MockBroker) GetOptionChainCtx(ctx context.Context, symbol, expiration string, withGreeks bool) ([]broker.Option, error) {
	args := m.Called(ctx, symbol, expiration, withGreeks)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]broker.Option), args.Error(1)
}

func (m *MockBroker) PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string, quantity int, price float64, preview bool, duration, clientOrderID string) (*broker.OrderResponse, error) {
	args := m.Called(symbol, putStrike, callStrike, expiration, quantity, price, preview, duration, clientOrderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string, quantity int, price, profitTarget float64, preview bool, duration string, tag string) (*broker.OrderResponse, error) {
	args := m.Called(symbol, putStrike, callStrike, expiration, quantity, price, profitTarget, preview, duration, tag)
	v := args.Get(0)
	if v == nil {
		return nil, args.Error(1)
	}
	resp, _ := v.(*broker.OrderResponse)
	return resp, args.Error(1)
}

func (m *MockBroker) GetOrderStatus(orderID int) (*broker.OrderResponse, error) {
	args := m.Called(orderID)
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) GetOrderStatusCtx(ctx context.Context, orderID int) (*broker.OrderResponse, error) {
	args := m.Called(ctx, orderID)
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) CancelOrder(orderID int) (*broker.OrderResponse, error) {
	args := m.Called(orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) CancelOrderCtx(ctx context.Context, orderID int) (*broker.OrderResponse, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) PlaceBuyToCloseOrder(optionSymbol string, quantity int, maxPrice float64, duration string, tag string) (*broker.OrderResponse, error) {
	args := m.Called(optionSymbol, quantity, maxPrice, duration, tag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string, quantity int, maxDebit float64, tag string) (*broker.OrderResponse, error) {
	args := m.Called(symbol, putStrike, callStrike, expiration, quantity, maxDebit, tag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) CloseStranglePositionCtx(ctx context.Context, symbol string, putStrike, callStrike float64, expiration string, quantity int, maxDebit float64, tag string) (*broker.OrderResponse, error) {
	args := m.Called(ctx, symbol, putStrike, callStrike, expiration, quantity, maxDebit, tag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) PlaceSellToCloseOrder(optionSymbol string, quantity int, maxPrice float64, duration string, tag string) (*broker.OrderResponse, error) {
	args := m.Called(optionSymbol, quantity, maxPrice, duration, tag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) PlaceBuyToCloseMarketOrder(optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	args := m.Called(optionSymbol, quantity, duration, tag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) PlaceSellToCloseMarketOrder(optionSymbol string, quantity int, duration string, tag string) (*broker.OrderResponse, error) {
	args := m.Called(optionSymbol, quantity, duration, tag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.OrderResponse), args.Error(1)
}

func (m *MockBroker) GetMarketClock(delayed bool) (*broker.MarketClockResponse, error) {
	args := m.Called(delayed)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.MarketClockResponse), args.Error(1)
}

func (m *MockBroker) IsTradingDay(delayed bool) (bool, error) {
	args := m.Called(delayed)
	return args.Get(0).(bool), args.Error(1)
}

func (m *MockBroker) GetTickSize(symbol string) (float64, error) {
	args := m.Called(symbol)
	return args.Get(0).(float64), args.Error(1)
}

func (m *MockBroker) GetHistoricalData(symbol, interval string, start, end time.Time) ([]broker.HistoricalDataPoint, error) {
	args := m.Called(symbol, interval, start, end)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]broker.HistoricalDataPoint), args.Error(1)
}

func (m *MockBroker) GetMarketCalendar(month, year int) (*broker.MarketCalendarResponse, error) {
	args := m.Called(month, year)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.MarketCalendarResponse), args.Error(1)
}

func (m *MockBroker) GetMarketCalendarCtx(ctx context.Context, month, year int) (*broker.MarketCalendarResponse, error) {
	args := m.Called(ctx, month, year)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*broker.MarketCalendarResponse), args.Error(1)
}

// TestBot creates a bot with mocked dependencies for testing
type TestBot struct {
	*Bot
	mockBroker  *MockBroker
	mockStorage *storage.MockStorage
	ctx         context.Context
	cancel      context.CancelFunc
}

// createTestBot creates a bot configured for testing
func createTestBot(t *testing.T) *TestBot {
	// Create mock dependencies
	mockBroker := NewMockBroker()
	mockStorage := storage.NewMockStorage()
	
	// Create test config
	cfg := &config.Config{
		Environment: config.EnvironmentConfig{
			Mode:     "paper",
			LogLevel: "debug",
		},
		Broker: config.BrokerConfig{
			APIKey:    "test-key",
			AccountID: "test-account",
			UseOTOCO:  true,
		},
		Strategy: config.StrategyConfig{
			Symbol:       "SPY",
			AllocationPct: 0.35,
			Entry: config.EntryConfig{
				TargetDTE:  45,
				DTERange:   []int{40, 50},
				Delta:      16,
				MinIVPct:   30,
				MinCredit:  2.0,
				MinVolume:  10,
				MinOpenInterest: 10,
			},
			Exit: config.ExitConfig{
				ProfitTarget: 0.5,
				MaxDTE:       21,
				StopLossPct:  2.5,
			},
			EscalateLossPct: 2.0, // 200% loss ratio
			MaxNewPositionsPerCycle: 1,
			Adjustments: config.AdjustmentConfig{
				Enabled: false,
			},
		},
		Risk: config.RiskConfig{
			MaxPositions:    1,
			MaxContracts:    10,
			MaxPositionLoss: 1000,
		},
		Schedule: config.ScheduleConfig{
			MarketCheckInterval: "30s",
			AfterHoursCheck:     false,
			TradingStart:        "09:30",
			TradingEnd:          "16:00",
			Timezone:            "America/New_York",
		},
		Storage: config.StorageConfig{
			Path: "/tmp/test_positions.json",
		},
	}
	
	// Create logger
	logger := log.New(io.Discard, "", 0)
	
	// Load NY timezone
	nyLocation, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	
	// Create bot
	bot := &Bot{
		config:        cfg,
		broker:        mockBroker,
		storage:       mockStorage,
		logger:        logger,
		stop:          make(chan struct{}),
		nyLocation:    nyLocation,
		pnlThrottle:   30 * time.Second,
		lastPnLUpdate: time.Now().Add(-time.Hour),
	}
	
	// Initialize strategy
	strategyConfig := &strategy.Config{
		Symbol:              cfg.Strategy.Symbol,
		DTETarget:           cfg.Strategy.Entry.TargetDTE,
		DTERange:            []int{cfg.Strategy.Entry.DTERange[0], cfg.Strategy.Entry.DTERange[1]},
		DeltaTarget:         float64(cfg.Strategy.Entry.Delta) / 100,
		ProfitTarget:        cfg.Strategy.Exit.ProfitTarget,
		MaxDTE:              cfg.Strategy.Exit.MaxDTE,
		AllocationPct:       cfg.Strategy.AllocationPct,
		MinIVPct:            cfg.Strategy.Entry.MinIVPct,
		MinCredit:           cfg.Strategy.Entry.MinCredit,
		EscalateLossPct:     cfg.Strategy.EscalateLossPct,
		StopLossPct:         cfg.Strategy.Exit.StopLossPct,
		MaxPositionLoss:     cfg.Risk.MaxPositionLoss,
		MaxContracts:        cfg.Risk.MaxContracts,
		MinVolume:           cfg.Strategy.Entry.MinVolume,
		MinOpenInterest:     cfg.Strategy.Entry.MinOpenInterest,
	}
	bot.strategy = strategy.NewStrangleStrategy(mockBroker, strategyConfig, logger, mockStorage)
	
	// Initialize order manager
	bot.orderManager = orders.NewManager(mockBroker, mockStorage, logger, bot.stop)
	
	// Initialize retry client
	bot.retryClient = retry.NewClient(mockBroker, logger)
	
	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	bot.ctx = ctx
	
	return &TestBot{
		Bot:         bot,
		mockBroker:  mockBroker,
		mockStorage: mockStorage,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func TestRunTradingCycle_MarketClosed(t *testing.T) {
	tb := createTestBot(t)
	defer tb.cancel()
	
	// Setup mock expectations for market closed
	tb.mockBroker.On("GetMarketCalendarCtx", mock.Anything, mock.Anything, mock.Anything).Return(&broker.MarketCalendarResponse{
		Calendar: struct {
			Month int `json:"month"`
			Year  int `json:"year"`
			Days  struct {
				Day []broker.MarketDay `json:"day"`
			} `json:"days"`
		}{
			Days: struct {
				Day []broker.MarketDay `json:"day"`
			}{
				Day: []broker.MarketDay{
					{
						Date:        time.Now().Format("2006-01-02"),
						Status:      "closed",
						Description: "Weekend",
					},
				},
			},
		},
	}, nil)
	
	tb.mockBroker.On("GetMarketClock", false).Return(&broker.MarketClockResponse{
		Clock: struct {
			Date        string `json:"date"`
			Description string `json:"description"`
			State       string `json:"state"`
			Timestamp   int64  `json:"timestamp"`
			NextChange  string `json:"next_change"`
			NextState   string `json:"next_state"`
		}{
			State: "closed",
		},
	}, nil)
	
	// Run trading cycle
	tradingCycle := NewTradingCycle(tb.Bot)
	tradingCycle.Run()
	
	// Verify no positions were checked or opened
	tb.mockBroker.AssertNotCalled(t, "GetPositions")
	tb.mockBroker.AssertNotCalled(t, "PlaceStrangleOrder")
	tb.mockBroker.AssertNotCalled(t, "PlaceStrangleOTOCO")
}

func TestBot_GracefulShutdown(t *testing.T) {
	tb := createTestBot(t)
	
	// Setup broker connection check
	tb.mockBroker.On("GetAccountBalanceCtx", mock.Anything).Return(10000.0, nil)
	
	// Setup market calendar (will be called during trading cycle check)
	tb.mockBroker.On("GetMarketCalendarCtx", mock.Anything, mock.Anything, mock.Anything).Return(&broker.MarketCalendarResponse{
		Calendar: struct {
			Month int `json:"month"`
			Year  int `json:"year"`
			Days  struct {
				Day []broker.MarketDay `json:"day"`
			} `json:"days"`
		}{
			Days: struct {
				Day []broker.MarketDay `json:"day"`
			}{
				Day: []broker.MarketDay{
					{
						Date:        time.Now().Format("2006-01-02"),
						Status:      "closed",
						Description: "Weekend",
					},
				},
			},
		},
	}, nil).Maybe()
	
	// Start bot in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- tb.Run(tb.ctx)
	}()
	
	// Give bot time to start
	time.Sleep(100 * time.Millisecond)
	
	// Trigger shutdown
	close(tb.stop)
	tb.cancel()
	
	// Wait for bot to stop
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Bot did not shut down within timeout")
	}
}

func TestTradingCycle_Components(t *testing.T) {
	tb := createTestBot(t)
	defer tb.cancel()
	
	// Test TradingCycle creation
	tradingCycle := NewTradingCycle(tb.Bot)
	assert.NotNil(t, tradingCycle)
	assert.NotNil(t, tradingCycle.bot)
	assert.NotNil(t, tradingCycle.reconciler)
}

func TestReconciler_Components(t *testing.T) {
	tb := createTestBot(t)
	defer tb.cancel()
	
	// Test Reconciler creation
	reconciler := NewReconciler(tb.mockBroker, tb.mockStorage, tb.logger)
	assert.NotNil(t, reconciler)
	assert.Equal(t, tb.mockBroker, reconciler.broker)
	assert.Equal(t, tb.mockStorage, reconciler.storage)
	assert.Equal(t, tb.logger, reconciler.logger)
}

func TestReconcilePositions_EmptyPositions(t *testing.T) {
	tb := createTestBot(t)
	defer tb.cancel()
	
	// No stored positions
	storedPositions := []models.Position{}
	
	// Broker has no positions
	tb.mockBroker.On("GetPositions").Return([]broker.PositionItem{}, nil)
	
	// Run reconciliation
	reconciler := NewReconciler(tb.mockBroker, tb.mockStorage, tb.logger)
	activePositions := reconciler.ReconcilePositions(storedPositions)
	
	// Verify no positions after reconciliation
	assert.Len(t, activePositions, 0)
}

