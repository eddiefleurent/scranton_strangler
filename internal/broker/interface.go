package broker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/sony/gobreaker"
)

// Broker defines the interface for interacting with a brokerage
type Broker interface {
	// Account operations
	GetAccountBalance() (float64, error)
	GetAccountBalanceCtx(ctx context.Context) (float64, error)
	GetOptionBuyingPower() (float64, error)
	GetPositions() ([]PositionItem, error)

	// Market data
	GetQuote(symbol string) (*QuoteItem, error)
	GetExpirations(symbol string) ([]string, error)
	GetExpirationsCtx(ctx context.Context, symbol string) ([]string, error)
	GetOptionChain(symbol, expiration string, withGreeks bool) ([]Option, error)
	GetOptionChainCtx(ctx context.Context, symbol, expiration string, withGreeks bool) ([]Option, error)
	GetMarketClock(delayed bool) (*MarketClockResponse, error)
	GetMarketCalendar(month, year int) (*MarketCalendarResponse, error)
	IsTradingDay(delayed bool) (bool, error)
	GetTickSize(symbol string) (float64, error) // Get appropriate tick size for symbol
	GetHistoricalData(symbol string, interval string, startDate, endDate time.Time) ([]HistoricalDataPoint, error)

	// Order placement
	// PlaceStrangleOrder: limitPrice is the total credit/debit limit for the entire strangle (per spread)
	// PlaceStrangleOTOCO: credit is the target credit amount, profitTarget is the profit ratio (0.0-1.0)
	PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string,
		quantity int, limitPrice float64, preview bool, duration string, tag string) (*OrderResponse, error)
	PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string,
		quantity int, credit, profitTarget float64, preview bool) (*OrderResponse, error)

	// Order status
	GetOrderStatus(orderID int) (*OrderResponse, error)
	GetOrderStatusCtx(ctx context.Context, orderID int) (*OrderResponse, error)

	// Position closing
	CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string,
		quantity int, maxDebit float64, tag string) (*OrderResponse, error)
	CloseStranglePositionCtx(ctx context.Context, symbol string, putStrike, callStrike float64, expiration string,
		quantity int, maxDebit float64, tag string) (*OrderResponse, error)
	PlaceBuyToCloseOrder(optionSymbol string, quantity int,
		maxPrice float64, duration string) (*OrderResponse, error)
	PlaceSellToCloseOrder(optionSymbol string, quantity int,
		maxPrice float64, duration string) (*OrderResponse, error)
	PlaceBuyToCloseMarketOrder(optionSymbol string, quantity int,
		duration string) (*OrderResponse, error)
	PlaceSellToCloseMarketOrder(optionSymbol string, quantity int,
		duration string) (*OrderResponse, error)
}

// TradierClient wraps TradierAPI to implement the Broker interface
type TradierClient struct {
	*TradierAPI
	useOTOCO     bool    // Configuration for whether to use OTOCO orders
	profitTarget float64 // Configurable profit target for OTOCO orders
}

// isNotImplementedError checks if an error indicates that OTOCO is not implemented (HTTP 501)
func isNotImplementedError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status == 501
	}
	return false
}

// Ensure TradierClient implements Broker at compile time.
var _ Broker = (*TradierClient)(nil)

// TradierClientOption represents a functional option for configuring TradierClient
type TradierClientOption func(*TradierClientConfig)

// TradierClientConfig holds configuration options for TradierClient
type TradierClientConfig struct {
	httpClient *http.Client
}

// WithHTTPClient sets a custom HTTP client for the TradierClient
func WithHTTPClient(client *http.Client) TradierClientOption {
	return func(config *TradierClientConfig) {
		config.httpClient = client
	}
}

// WithTransport sets a custom HTTP transport for the TradierClient
func WithTransport(transport http.RoundTripper) TradierClientOption {
	return func(config *TradierClientConfig) {
		if config.httpClient == nil {
			config.httpClient = &http.Client{Transport: transport, Timeout: 10 * time.Second}
		} else {
			config.httpClient.Transport = transport
		}
	}
}

// NewTradierClient creates a new Tradier broker client
// profitTarget should be a ratio between 0.0 and 1.0 (e.g., 0.5 for 50% profit target)
func NewTradierClient(apiKey, accountID string, sandbox bool,
	useOTOCO bool, profitTarget float64, opts ...TradierClientOption) (*TradierClient, error) {
	if profitTarget < 0 || profitTarget > 1 {
		return nil, fmt.Errorf("profitTarget %.3f is outside valid range [0.0, 1.0]", profitTarget)
	}

	// Apply functional options
	config := &TradierClientConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Create TradierAPI with custom client if provided
	var tradierAPI *TradierAPI
	if config.httpClient != nil {
		tradierAPI = NewTradierAPIWithClient(apiKey, accountID, sandbox, config.httpClient)
	} else {
		tradierAPI = NewTradierAPI(apiKey, accountID, sandbox)
	}

	return &TradierClient{
		TradierAPI:   tradierAPI,
		useOTOCO:     useOTOCO,
		profitTarget: profitTarget,
	}, nil
}

// GetAccountBalance returns the total account equity
func (t *TradierClient) GetAccountBalance() (float64, error) {
	balance, err := t.GetBalance()
	if err != nil {
		return 0, err
	}
	return balance.Balances.TotalEquity, nil
}

// GetAccountBalanceCtx returns the total account equity with context support
func (t *TradierClient) GetAccountBalanceCtx(ctx context.Context) (float64, error) {
	balance, err := t.GetBalanceCtx(ctx)
	if err != nil {
		return 0, err
	}
	return balance.Balances.TotalEquity, nil
}

// GetOptionBuyingPower returns the option buying power available for options trading
func (t *TradierClient) GetOptionBuyingPower() (float64, error) {
	balance, err := t.GetBalance()
	if err != nil {
		return 0, err
	}
	return balance.GetOptionBuyingPower()
}

// PlaceStrangleOrder places a strangle order, using OTOCO if configured
func (t *TradierClient) PlaceStrangleOrder(symbol string, putStrike, callStrike float64,
	expiration string, quantity int, limitPrice float64, preview bool, duration string, tag string) (*OrderResponse, error) {
	if quantity <= 0 {
		return nil, fmt.Errorf("quantity must be > 0")
	}
	if limitPrice < 0 {
		return nil, fmt.Errorf("limitPrice must be >= 0")
	}
	if t.useOTOCO {
		// Try OTOCO order with configurable profit target
		orderResp, err := t.TradierAPI.PlaceStrangleOTOCO(symbol, putStrike, callStrike,
			expiration, quantity, limitPrice, t.profitTarget, preview)
		if err != nil {
			// Only fall back to regular strangle order for explicit OTOCO unsupported signals
			if errors.Is(err, ErrOTOCOUnsupported) || isNotImplementedError(err) {
				// Log when OTOCO is explicitly unsupported and fall back to regular order
				log.Printf("warning: OTOCO unsupported (explicit signal), falling back to regular multileg: %v", err)
				// Use regular strangle order as fallback
				return t.TradierAPI.PlaceStrangleOrder(symbol, putStrike, callStrike,
					expiration, quantity, limitPrice, preview, duration, tag)
			}
			// Return the original error for all other cases without attempting fallback
			return nil, err
		}
		return orderResp, nil
	}
	// Use regular strangle order
	return t.TradierAPI.PlaceStrangleOrder(symbol, putStrike, callStrike,
		expiration, quantity, limitPrice, preview, duration, tag)
}

// PlaceStrangleOTOCO implements the Broker interface for OTOCO orders
func (t *TradierClient) PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64,
	expiration string, quantity int, credit, profitTarget float64, preview bool) (*OrderResponse, error) {
	if quantity <= 0 {
		return nil, fmt.Errorf("quantity must be > 0")
	}
	if credit < 0 {
		return nil, fmt.Errorf("credit must be >= 0")
	}
	if profitTarget < 0 || profitTarget > 1 {
		return nil, fmt.Errorf("profitTarget %.3f is outside valid range [0.0, 1.0]", profitTarget)
	}
	return t.TradierAPI.PlaceStrangleOTOCO(symbol, putStrike, callStrike,
		expiration, quantity, credit, profitTarget, preview)
}

// CloseStranglePosition closes an existing strangle position with a buy-to-close order
func (t *TradierClient) CloseStranglePosition(symbol string, putStrike, callStrike float64,
	expiration string, quantity int, maxDebit float64, tag string) (*OrderResponse, error) {
	return t.PlaceStrangleBuyToClose(symbol, putStrike, callStrike,
		expiration, quantity, maxDebit, string(DurationDay), tag)
}

// CloseStranglePositionCtx closes an existing strangle position with a buy-to-close order with context support
func (t *TradierClient) CloseStranglePositionCtx(ctx context.Context, symbol string, putStrike, callStrike float64,
	expiration string, quantity int, maxDebit float64, tag string) (*OrderResponse, error) {
	return t.PlaceStrangleBuyToCloseCtx(ctx, symbol, putStrike, callStrike,
		expiration, quantity, maxDebit, string(DurationDay), tag)
}

// GetOrderStatus retrieves the status of an existing order
func (t *TradierClient) GetOrderStatus(orderID int) (*OrderResponse, error) {
	return t.TradierAPI.GetOrderStatus(orderID)
}

// GetOrderStatusCtx retrieves the status of an existing order with context
func (t *TradierClient) GetOrderStatusCtx(ctx context.Context, orderID int) (*OrderResponse, error) {
	return t.TradierAPI.GetOrderStatusCtx(ctx, orderID)
}

// PlaceBuyToCloseOrder places a buy-to-close order for a specific option
func (t *TradierClient) PlaceBuyToCloseOrder(optionSymbol string, quantity int,
	maxPrice float64, duration string) (*OrderResponse, error) {
	return t.TradierAPI.PlaceBuyToCloseOrder(optionSymbol, quantity, maxPrice, duration)
}

// PlaceSellToCloseOrder places a sell-to-close order for a specific option
func (t *TradierClient) PlaceSellToCloseOrder(optionSymbol string, quantity int,
	maxPrice float64, duration string) (*OrderResponse, error) {
	return t.TradierAPI.PlaceSellToCloseOrder(optionSymbol, quantity, maxPrice, duration)
}

// PlaceBuyToCloseMarketOrder places a buy-to-close market order for a specific option
func (t *TradierClient) PlaceBuyToCloseMarketOrder(optionSymbol string, quantity int,
	duration string) (*OrderResponse, error) {
	return t.TradierAPI.PlaceBuyToCloseMarketOrder(optionSymbol, quantity, duration)
}

// PlaceSellToCloseMarketOrder places a sell-to-close market order for a specific option
func (t *TradierClient) PlaceSellToCloseMarketOrder(optionSymbol string, quantity int,
	duration string) (*OrderResponse, error) {
	return t.TradierAPI.PlaceSellToCloseMarketOrder(optionSymbol, quantity, duration)
}

// GetMarketClock retrieves the current market clock status
func (t *TradierClient) GetMarketClock(delayed bool) (*MarketClockResponse, error) {
	return t.TradierAPI.GetMarketClock(delayed)
}

// GetMarketCalendar retrieves the market calendar for a specific month/year
func (t *TradierClient) GetMarketCalendar(month, year int) (*MarketCalendarResponse, error) {
	return t.TradierAPI.GetMarketCalendar(month, year)
}

// IsTradingDay checks if the market is currently open for trading
func (t *TradierClient) IsTradingDay(delayed bool) (bool, error) {
	return t.TradierAPI.IsTradingDay(delayed)
}

// GetHistoricalData retrieves historical price data for a symbol
func (t *TradierClient) GetHistoricalData(symbol string, interval string, startDate, endDate time.Time) ([]HistoricalDataPoint, error) {
	return t.TradierAPI.GetHistoricalData(symbol, interval, startDate, endDate)
}

// GetTickSize returns the appropriate tick size for the given symbol
// Most US stocks and ETFs trade in penny increments (0.01)
// Some lower-priced stocks may trade in 0.0001 increments
func (t *TradierClient) GetTickSize(symbol string) (float64, error) {
	// Most US equities/options trade in $0.01 increments; callers can override if needed.
	return 0.01, nil
}

// CalculateIVR calculates Implied Volatility Rank from historical data
func CalculateIVR(currentIV float64, historicalIVs []float64) float64 {
	if math.IsNaN(currentIV) {
		return 0
	}
	if math.IsInf(currentIV, 1) { // Positive infinity
		return 100
	}
	if math.IsInf(currentIV, -1) { // Negative infinity
		return 0
	}

	// Filter invalid historical values
	clean := make([]float64, 0, len(historicalIVs))
	for _, v := range historicalIVs {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			clean = append(clean, v)
		}
	}

	if len(clean) == 0 {
		return 0
	}

	// Find min and max IV over the period
	minIV := clean[0]
	maxIV := clean[0]

	for _, iv := range clean {
		if iv < minIV {
			minIV = iv
		}
		if iv > maxIV {
			maxIV = iv
		}
	}

	// IVR = (Current IV - period low) / (period high - period low) * 100
	if maxIV == minIV {
		return 0
	}
	r := ((currentIV - minIV) / (maxIV - minIV)) * 100
	// IVR should be bounded between 0-100
	if r < 0 {
		return 0
	}
	if r > 100 {
		return 100
	}
	return r
}

// GetOptionByStrike finds an option with a specific strike price
// Note: Option.OptionType is defined as string for JSON compatibility,
// so we convert optionType (OptionType) to string for comparison
func GetOptionByStrike(options []Option, strike float64, optionType OptionType) *Option {
	for i := range options {
		if math.Abs(options[i].Strike-strike) <= 1e-4 && options[i].OptionType == string(optionType) {
			return &options[i]
		}
	}
	return nil
}

// OptionTypeMatches checks if an option's type matches the expected type
// This helper avoids brittle string casting and centralizes option type comparisons
func OptionTypeMatches(optionType string, expectedType OptionType) bool {
	return optionType == string(expectedType)
}

// OptionType represents the type of option contract
type OptionType string

const (
	// OptionTypePut represents a put option contract
	OptionTypePut OptionType = "put"
	// OptionTypeCall represents a call option contract
	OptionTypeCall OptionType = "call"
)

// DurationType represents order duration types
type DurationType string

const (
	// DurationDay represents a day order duration
	DurationDay DurationType = "day"
)

// Use OptionTypePut/OptionTypeCall everywhere to avoid duplication.

// AbsDaysBetween calculates the absolute number of days between two dates
func AbsDaysBetween(from, to time.Time) int {
	f := from.UTC().Truncate(24 * time.Hour)
	t := to.UTC().Truncate(24 * time.Hour)
	d := int(t.Sub(f).Hours() / 24)
	if d < 0 {
		return -d
	}
	return d
}

// CircuitBreakerBroker wraps a Broker with circuit breaker functionality
type CircuitBreakerBroker struct {
	broker  Broker
	breaker *gobreaker.CircuitBreaker
}

// exec is a generic helper for circuit breaker wrapper methods
func execCircuitBreaker[T any](
	breaker *gobreaker.CircuitBreaker,
	broker Broker,
	fn func(Broker) (T, error),
) (T, error) {
	var zero T
	res, err := breaker.Execute(func() (interface{}, error) { return fn(broker) })
	if err != nil {
		return zero, err
	}
	if res == nil {
		return zero, nil
	}
	v, ok := res.(T)
	if !ok {
		return zero, errors.New("circuit breaker: type assertion failed")
	}
	return v, nil
}

// NewCircuitBreakerBroker creates a new CircuitBreakerBroker with sensible defaults
func NewCircuitBreakerBroker(broker Broker) *CircuitBreakerBroker {
	return NewCircuitBreakerBrokerWithSettings(broker, CircuitBreakerSettings{
		MaxRequests:  3,                // Allow 3 requests when half-open
		Interval:     60 * time.Second, // Reset counts every minute
		Timeout:      30 * time.Second, // Open circuit for 30 seconds
		MinRequests:  5,                // Minimum requests before tripping
		FailureRatio: 0.6,              // Trip if 60% failure rate
	})
}

// stateName returns human-friendly name for circuit breaker state
func stateName(s gobreaker.State) string {
	switch s {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateOpen:
		return "open"
	case gobreaker.StateHalfOpen:
		return "half-open"
	default:
		return fmt.Sprintf("%v", s)
	}
}

// CircuitBreakerSettings configures circuit breaker behavior
type CircuitBreakerSettings struct {
	MaxRequests  uint32        // Max requests when half-open
	Interval     time.Duration // Reset counts interval
	Timeout      time.Duration // Open circuit duration
	MinRequests  uint32        // Min requests before tripping
	FailureRatio float64       // Failure ratio threshold
	Logger       *log.Logger   // Optional logger (uses log.Default() if nil)
}

// NewCircuitBreakerBrokerWithSettings creates a CircuitBreakerBroker with custom settings
func NewCircuitBreakerBrokerWithSettings(broker Broker, settings CircuitBreakerSettings) *CircuitBreakerBroker {
	gbSettings := gobreaker.Settings{
		Name:        "BrokerCircuitBreaker",
		MaxRequests: settings.MaxRequests,
		Interval:    settings.Interval,
		Timeout:     settings.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests == 0 || counts.Requests < settings.MinRequests {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= settings.FailureRatio
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger := settings.Logger
			if logger == nil {
				logger = log.Default()
			}
			logger.Printf("Circuit breaker %s state changed from %s to %s", name, stateName(from), stateName(to))
		},
	}

	return &CircuitBreakerBroker{
		broker:  broker,
		breaker: gobreaker.NewCircuitBreaker(gbSettings),
	}
}

// GetAccountBalance wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetAccountBalance() (float64, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (float64, error) { return b.GetAccountBalance() })
}

// GetAccountBalanceCtx wraps the underlying broker call with circuit breaker and context
func (c *CircuitBreakerBroker) GetAccountBalanceCtx(ctx context.Context) (float64, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (float64, error) {
		return b.GetAccountBalanceCtx(ctx)
	})
}

// GetOptionBuyingPower wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetOptionBuyingPower() (float64, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (float64, error) { return b.GetOptionBuyingPower() })
}

// GetPositions wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetPositions() ([]PositionItem, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) ([]PositionItem, error) { return b.GetPositions() })
}

// GetQuote wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetQuote(symbol string) (*QuoteItem, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*QuoteItem, error) { return b.GetQuote(symbol) })
}

// GetExpirations wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetExpirations(symbol string) ([]string, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) ([]string, error) { return b.GetExpirations(symbol) })
}

// GetExpirationsCtx wraps the underlying broker call with circuit breaker and context support
func (c *CircuitBreakerBroker) GetExpirationsCtx(ctx context.Context, symbol string) ([]string, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) ([]string, error) {
		return b.GetExpirationsCtx(ctx, symbol)
	})
}

// GetOptionChain wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetOptionChain(symbol, expiration string, withGreeks bool) ([]Option, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) ([]Option, error) {
		return b.GetOptionChain(symbol, expiration, withGreeks)
	})
}

// GetOptionChainCtx wraps the underlying broker call with circuit breaker and context
func (c *CircuitBreakerBroker) GetOptionChainCtx(ctx context.Context, symbol, expiration string, withGreeks bool) ([]Option, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) ([]Option, error) {
		return b.GetOptionChainCtx(ctx, symbol, expiration, withGreeks)
	})
}

// PlaceStrangleOrder wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string,
	quantity int, limitPrice float64, preview bool, duration string, tag string) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.PlaceStrangleOrder(symbol, putStrike, callStrike, expiration, quantity, limitPrice, preview, duration, tag)
	})
}

// PlaceStrangleOTOCO wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string,
	quantity int, credit, profitTarget float64, preview bool) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.PlaceStrangleOTOCO(symbol, putStrike, callStrike, expiration, quantity, credit, profitTarget, preview)
	})
}

// GetOrderStatus wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetOrderStatus(orderID int) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.GetOrderStatus(orderID)
	})
}

// GetOrderStatusCtx wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetOrderStatusCtx(ctx context.Context, orderID int) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.GetOrderStatusCtx(ctx, orderID)
	})
}

// CloseStranglePosition wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string,
	quantity int, maxDebit float64, tag string) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.CloseStranglePosition(symbol, putStrike, callStrike, expiration, quantity, maxDebit, tag)
	})
}

// CloseStranglePositionCtx wraps the underlying broker call with circuit breaker and context support
func (c *CircuitBreakerBroker) CloseStranglePositionCtx(ctx context.Context, symbol string, putStrike, callStrike float64, expiration string,
	quantity int, maxDebit float64, tag string) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.CloseStranglePositionCtx(ctx, symbol, putStrike, callStrike, expiration, quantity, maxDebit, tag)
	})
}

// PlaceBuyToCloseOrder wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) PlaceBuyToCloseOrder(optionSymbol string, quantity int,
	maxPrice float64, duration string) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.PlaceBuyToCloseOrder(optionSymbol, quantity, maxPrice, duration)
	})
}

// PlaceSellToCloseOrder wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) PlaceSellToCloseOrder(optionSymbol string, quantity int,
	maxPrice float64, duration string) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.PlaceSellToCloseOrder(optionSymbol, quantity, maxPrice, duration)
	})
}

// PlaceBuyToCloseMarketOrder wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) PlaceBuyToCloseMarketOrder(optionSymbol string, quantity int,
	duration string) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.PlaceBuyToCloseMarketOrder(optionSymbol, quantity, duration)
	})
}

// PlaceSellToCloseMarketOrder wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) PlaceSellToCloseMarketOrder(optionSymbol string, quantity int,
	duration string) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.PlaceSellToCloseMarketOrder(optionSymbol, quantity, duration)
	})
}

// GetMarketClock wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetMarketClock(delayed bool) (*MarketClockResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*MarketClockResponse, error) {
		return b.GetMarketClock(delayed)
	})
}

// GetMarketCalendar wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetMarketCalendar(month, year int) (*MarketCalendarResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*MarketCalendarResponse, error) {
		return b.GetMarketCalendar(month, year)
	})
}

// GetHistoricalData wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetHistoricalData(symbol string, interval string, startDate, endDate time.Time) ([]HistoricalDataPoint, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) ([]HistoricalDataPoint, error) {
		return b.GetHistoricalData(symbol, interval, startDate, endDate)
	})
}

// IsTradingDay wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) IsTradingDay(delayed bool) (bool, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (bool, error) {
		return b.IsTradingDay(delayed)
	})
}

// GetTickSize wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetTickSize(symbol string) (float64, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (float64, error) {
		return b.GetTickSize(symbol)
	})
}
