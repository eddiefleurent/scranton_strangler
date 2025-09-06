package broker

import "time"

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
	PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string, quantity int, credit float64) (*OrderResponse, error)
	PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string, quantity int, credit, profitTarget float64) (*OrderResponse, error)
	
	// Position closing
	CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string, quantity int, maxDebit float64) (*OrderResponse, error)
	PlaceBuyToCloseOrder(optionSymbol string, quantity int, maxPrice float64) (*OrderResponse, error)
}

// TradierClient wraps TradierAPI to implement the Broker interface
type TradierClient struct {
	*TradierAPI
	useOTOCO bool // Configuration for whether to use OTOCO orders
}

// NewTradierClient creates a new Tradier broker client
func NewTradierClient(apiKey, accountID string, sandbox bool, useOTOCO bool) *TradierClient {
	return &TradierClient{
		TradierAPI: NewTradierAPI(apiKey, accountID, sandbox),
		useOTOCO:   useOTOCO,
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
func (t *TradierClient) PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string, quantity int, credit float64) (*OrderResponse, error) {
	if t.useOTOCO {
		// Use OTOCO order with 50% profit target
		return t.TradierAPI.PlaceStrangleOTOCO(symbol, putStrike, callStrike, expiration, quantity, credit, 0.5, false)
	}
	// Use regular strangle order
	return t.TradierAPI.PlaceStrangleOrder(symbol, putStrike, callStrike, expiration, quantity, credit, false)
}

// PlaceStrangleOTOCO implements the Broker interface for OTOCO orders
func (t *TradierClient) PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string, quantity int, credit, profitTarget float64) (*OrderResponse, error) {
	return t.TradierAPI.PlaceStrangleOTOCO(symbol, putStrike, callStrike, expiration, quantity, credit, profitTarget, false)
}

// CloseStranglePosition closes an existing strangle position with a buy-to-close order
func (t *TradierClient) CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string, quantity int, maxDebit float64) (*OrderResponse, error) {
	return t.TradierAPI.PlaceStrangleBuyToClose(symbol, putStrike, callStrike, expiration, quantity, maxDebit)
}

// PlaceBuyToCloseOrder places a buy-to-close order for a specific option
func (t *TradierClient) PlaceBuyToCloseOrder(optionSymbol string, quantity int, maxPrice float64) (*OrderResponse, error) {
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
	
	// IVR = (Current IV - 52 week low) / (52 week high - 52 week low) * 100
	if maxIV == minIV {
		return 50 // Default to middle if no range
	}
	
	return ((currentIV - minIV) / (maxIV - minIV)) * 100
}

// GetOptionByStrike finds an option with a specific strike price
func GetOptionByStrike(options []Option, strike float64, optionType string) *Option {
	for i := range options {
		if options[i].Strike == strike && options[i].OptionType == optionType {
			return &options[i]
		}
	}
	return nil
}

// DaysBetween calculates the number of days between two dates
func DaysBetween(from, to time.Time) int {
	return int(to.Sub(from).Hours() / 24)
}