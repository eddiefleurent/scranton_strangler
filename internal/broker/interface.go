package broker

import (
	"context"
	"errors"
	"log"
	"math"
	"time"

	"github.com/sony/gobreaker"
)

// Broker defines the interface for interacting with a brokerage
type Broker interface {
	// Account operations
	GetAccountBalance() (float64, error)
	GetPositions() ([]PositionItem, error)

	// Market data
	GetQuote(symbol string) (*QuoteItem, error)
	GetExpirations(symbol string) ([]string, error)
	GetOptionChain(symbol, expiration string, withGreeks bool) ([]Option, error)
	GetMarketClock(delayed bool) (*MarketClockResponse, error)
	IsTradingDay(delayed bool) (bool, error)

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
	PlaceBuyToCloseOrder(optionSymbol string, quantity int,
		maxPrice float64) (*OrderResponse, error)
}

// TradierClient wraps TradierAPI to implement the Broker interface
type TradierClient struct {
	*TradierAPI
	useOTOCO     bool    // Configuration for whether to use OTOCO orders
	profitTarget float64 // Configurable profit target for OTOCO orders
}

// isPermanentAPIError checks if an error is a permanent API error that should trigger fallback
func isPermanentAPIError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		// Consider 4xx errors as permanent (except 429 Too Many Requests which is retryable)
		return apiErr.Status >= 400 && apiErr.Status < 500 && apiErr.Status != 429
	}
	return false
}

// Ensure TradierClient implements Broker at compile time.
var _ Broker = (*TradierClient)(nil)

// NewTradierClient creates a new Tradier broker client
// profitTarget should be a ratio between 0.0 and 1.0 (e.g., 0.5 for 50% profit target)
func NewTradierClient(apiKey, accountID string, sandbox bool,
	useOTOCO bool, profitTarget float64) *TradierClient {
	if profitTarget < 0 {
		log.Printf("warning: profitTarget %.3f is below valid range [0,1]; clamping to 0.0", profitTarget)
		profitTarget = 0.0
	} else if profitTarget > 1 {
		log.Printf("warning: profitTarget %.3f is above valid range [0,1]; clamping to 1.0", profitTarget)
		profitTarget = 1.0
	}
	return &TradierClient{
		TradierAPI:   NewTradierAPI(apiKey, accountID, sandbox),
		useOTOCO:     useOTOCO,
		profitTarget: profitTarget,
	}
}

// GetAccountBalance returns the total account equity
func (t *TradierClient) GetAccountBalance() (float64, error) {
	balance, err := t.GetBalance()
	if err != nil {
		return 0, err
	}
	return balance.Balances.TotalEquity, nil
}

// PlaceStrangleOrder places a strangle order, using OTOCO if configured
func (t *TradierClient) PlaceStrangleOrder(symbol string, putStrike, callStrike float64,
	expiration string, quantity int, limitPrice float64, preview bool, duration string, tag string) (*OrderResponse, error) {
	if t.useOTOCO {
		// Try OTOCO order with configurable profit target
		orderResp, err := t.TradierAPI.PlaceStrangleOTOCO(symbol, putStrike, callStrike,
			expiration, quantity, limitPrice, t.profitTarget, preview)
		if err != nil {
			// Check if error indicates OTOCO unsupported or other permanent API errors - fall back to regular order
			if errors.Is(err, ErrOTOCOUnsupported) || isPermanentAPIError(err) {
				// Log OTOCO-specific error and fall back to regular order
				log.Printf("warning: OTOCO unavailable or permanent API error, falling back to regular multileg: %v", err)
				// Use regular strangle order as fallback
				return t.TradierAPI.PlaceStrangleOrder(symbol, putStrike, callStrike,
					expiration, quantity, limitPrice, preview, duration, tag)
			}
			// Return the original error if it's not permanent
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
	if profitTarget < 0 {
		log.Printf("warning: profitTarget %.3f is below valid range [0,1]; clamping to 0.0", profitTarget)
		profitTarget = 0
	} else if profitTarget > 1 {
		log.Printf("warning: profitTarget %.3f is above valid range [0,1]; clamping to 1.0", profitTarget)
		profitTarget = 1
	}
	return t.TradierAPI.PlaceStrangleOTOCO(symbol, putStrike, callStrike,
		expiration, quantity, credit, profitTarget, preview)
}

// CloseStranglePosition closes an existing strangle position with a buy-to-close order
func (t *TradierClient) CloseStranglePosition(symbol string, putStrike, callStrike float64,
	expiration string, quantity int, maxDebit float64, tag string) (*OrderResponse, error) {
	return t.PlaceStrangleBuyToClose(symbol, putStrike, callStrike,
		expiration, quantity, maxDebit, tag)
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
	maxPrice float64) (*OrderResponse, error) {
	return t.TradierAPI.PlaceBuyToCloseOrder(optionSymbol, quantity, maxPrice)
}

// GetMarketClock retrieves the current market clock status
func (t *TradierClient) GetMarketClock(delayed bool) (*MarketClockResponse, error) {
	return t.TradierAPI.GetMarketClock(delayed)
}

// IsTradingDay checks if the market is currently open for trading
func (t *TradierClient) IsTradingDay(delayed bool) (bool, error) {
	return t.TradierAPI.IsTradingDay(delayed)
}

// CalculateIVR calculates Implied Volatility Rank from historical data
func CalculateIVR(currentIV float64, historicalIVs []float64) float64 {
	if math.IsNaN(currentIV) || math.IsInf(currentIV, 0) {
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
	if r < 0 {
		return 0
	}
	if r > 100 {
		return 100
	}
	return r
}

// GetOptionByStrike finds an option with a specific strike price
func GetOptionByStrike(options []Option, strike float64, optionType OptionType) *Option {
	for i := range options {
		if math.Abs(options[i].Strike-strike) <= 1e-4 && options[i].OptionType == string(optionType) {
			return &options[i]
		}
	}
	return nil
}

// OptionType represents the type of option contract
type OptionType string

const (
	// OptionTypePut represents a put option contract
	OptionTypePut OptionType = "put"
	// OptionTypeCall represents a call option contract
	OptionTypeCall OptionType = "call"
)

// DaysBetween calculates the number of days between two dates
func DaysBetween(from, to time.Time) int {
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

// CircuitBreakerSettings configures circuit breaker behavior
type CircuitBreakerSettings struct {
	MaxRequests  uint32        // Max requests when half-open
	Interval     time.Duration // Reset counts interval
	Timeout      time.Duration // Open circuit duration
	MinRequests  uint32        // Min requests before tripping
	FailureRatio float64       // Failure ratio threshold
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
			log.Printf("Circuit breaker %s state changed from %s to %s", name, from, to)
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

// GetOptionChain wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetOptionChain(symbol, expiration string, withGreeks bool) ([]Option, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) ([]Option, error) {
		return b.GetOptionChain(symbol, expiration, withGreeks)
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

// PlaceBuyToCloseOrder wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) PlaceBuyToCloseOrder(optionSymbol string, quantity int,
	maxPrice float64) (*OrderResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*OrderResponse, error) {
		return b.PlaceBuyToCloseOrder(optionSymbol, quantity, maxPrice)
	})
}

// GetMarketClock wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) GetMarketClock(delayed bool) (*MarketClockResponse, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (*MarketClockResponse, error) {
		return b.GetMarketClock(delayed)
	})
}

// IsTradingDay wraps the underlying broker call with circuit breaker
func (c *CircuitBreakerBroker) IsTradingDay(delayed bool) (bool, error) {
	return execCircuitBreaker(c.breaker, c.broker, func(b Broker) (bool, error) {
		return b.IsTradingDay(delayed)
	})
}
