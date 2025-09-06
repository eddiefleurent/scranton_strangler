package broker

import (
	"context"
	"log"
	"math"
	"strings"
	"time"
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

	// Order placement
	PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string,
		quantity int, limitPrice float64, preview bool) (*OrderResponse, error)
	PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string,
		quantity int, credit, profitTarget float64) (*OrderResponse, error)

	// Order status
	GetOrderStatus(orderID int) (*OrderResponse, error)
	GetOrderStatusCtx(ctx context.Context, orderID int) (*OrderResponse, error)

	// Position closing
	CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string,
		quantity int, maxDebit float64) (*OrderResponse, error)
	PlaceBuyToCloseOrder(optionSymbol string, quantity int,
		maxPrice float64) (*OrderResponse, error)
}

// TradierClient wraps TradierAPI to implement the Broker interface
type TradierClient struct {
	*TradierAPI
	useOTOCO     bool    // Configuration for whether to use OTOCO orders
	profitTarget float64 // Configurable profit target for OTOCO orders
}

// NewTradierClient creates a new Tradier broker client
// profitTarget should be a ratio between 0.0 and 1.0 (e.g., 0.5 for 50% profit target)
func NewTradierClient(apiKey, accountID string, sandbox bool,
	useOTOCO bool, profitTarget float64) *TradierClient {
	if profitTarget < 0 || profitTarget > 1 {
		// Reset to 0 instead of panicking - let caller handle invalid values
		profitTarget = 0
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
	expiration string, quantity int, limitPrice float64, preview bool) (*OrderResponse, error) {
	if t.useOTOCO {
		// Try OTOCO order with configurable profit target
		orderResp, err := t.TradierAPI.PlaceStrangleOTOCO(symbol, putStrike, callStrike,
			expiration, quantity, limitPrice, t.profitTarget, preview)
		if err != nil {
			// Check if error indicates OTOCO unsupported - fall back to regular order
			if strings.Contains(err.Error(), "unsupported") || strings.Contains(err.Error(), "OTOCO") {
				// Log OTOCO-specific error and fall back to regular order
				log.Printf("warning: OTOCO unavailable, falling back to regular multileg: %v", err)
				// Use regular strangle order as fallback
				return t.TradierAPI.PlaceStrangleOrder(symbol, putStrike, callStrike,
					expiration, quantity, limitPrice, preview)
			}
			// Return the original error if it's not OTOCO-related
			return nil, err
		}
		return orderResp, nil
	}
	// Use regular strangle order
	return t.TradierAPI.PlaceStrangleOrder(symbol, putStrike, callStrike,
		expiration, quantity, limitPrice, preview)
}

// PlaceStrangleOTOCO implements the Broker interface for OTOCO orders
func (t *TradierClient) PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64,
	expiration string, quantity int, credit, profitTarget float64) (*OrderResponse, error) {
	return t.TradierAPI.PlaceStrangleOTOCO(symbol, putStrike, callStrike,
		expiration, quantity, credit, profitTarget, false)
}

// CloseStranglePosition closes an existing strangle position with a buy-to-close order
func (t *TradierClient) CloseStranglePosition(symbol string, putStrike, callStrike float64,
	expiration string, quantity int, maxDebit float64) (*OrderResponse, error) {
	return t.TradierAPI.PlaceStrangleBuyToClose(symbol, putStrike, callStrike,
		expiration, quantity, maxDebit)
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

// CalculateIVR calculates Implied Volatility Rank from historical data
func CalculateIVR(currentIV float64, historicalIVs []float64) float64 {
	if len(historicalIVs) == 0 {
		return 0
	}

	// Find min and max IV over the period
	minIV := historicalIVs[0]
	maxIV := historicalIVs[0]

	for _, iv := range historicalIVs {
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
func GetOptionByStrike(options []Option, strike float64, optionType string) *Option {
	for i := range options {
		if math.Abs(options[i].Strike-strike) < 1e-6 && options[i].OptionType == optionType {
			return &options[i]
		}
	}
	return nil
}

// DaysBetween calculates the number of days between two dates
func DaysBetween(from, to time.Time) int {
	f := from.UTC().Truncate(24 * time.Hour)
	t := to.UTC().Truncate(24 * time.Hour)
	return int(t.Sub(f).Hours() / 24)
}
