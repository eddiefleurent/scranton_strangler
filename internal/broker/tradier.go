package broker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TradierAPI - Accurate implementation based on actual API docs
type TradierAPI struct {
	apiKey      string
	baseURL     string
	accountID   string
	client      *http.Client
	sandbox     bool
	rateLimits  RateLimits
}

// Rate limits per environment
type RateLimits struct {
	MarketData int // requests per minute
	Trading    int // requests per minute
	Standard   int // requests per minute
}

func NewTradierAPI(apiKey, accountID string, sandbox bool) *TradierAPI {
	var baseURL string
	var limits RateLimits
	
	if sandbox {
		baseURL = "https://sandbox.tradier.com/v1"
		limits = RateLimits{
			MarketData: 60,
			Trading:    60,
			Standard:   60,
		}
	} else {
		baseURL = "https://api.tradier.com/v1"
		limits = RateLimits{
			MarketData: 120,
			Trading:    60,
			Standard:   120,
		}
	}
	
	return &TradierAPI{
		apiKey:     apiKey,
		baseURL:    baseURL,
		accountID:  accountID,
		client:     &http.Client{Timeout: 10 * time.Second},
		sandbox:    sandbox,
		rateLimits: limits,
	}
}

// ============ EXACT API Response Structures ============

// Option chain response - matches API exactly
type OptionChainResponse struct {
	Options struct {
		Option []Option `json:"option"`
	} `json:"options"`
}

type Option struct {
	Symbol         string    `json:"symbol"`         // e.g., "SPY241220P00450000"
	Description    string    `json:"description"`    // e.g., "SPY Dec 20 2024 $450.00 Put"
	Strike         float64   `json:"strike"`         // Strike price
	OptionType     string    `json:"option_type"`    // "put" or "call"
	ExpirationDate string    `json:"expiration_date"`
	ExpirationDay  int       `json:"expiration_day"`
	Bid            float64   `json:"bid"`
	Ask            float64   `json:"ask"`
	Last           float64   `json:"last"`
	BidSize        int       `json:"bid_size"`
	AskSize        int       `json:"ask_size"`
	Volume         int64     `json:"volume"`
	OpenInterest   int64     `json:"open_interest"`
	Underlying     string    `json:"underlying"`
	Greeks         *Greeks   `json:"greeks,omitempty"`
}

type Greeks struct {
	Delta     float64 `json:"delta"`
	Gamma     float64 `json:"gamma"`
	Theta     float64 `json:"theta"`
	Vega      float64 `json:"vega"`
	Rho       float64 `json:"rho"`
	Phi       float64 `json:"phi"`
	BidIV     float64 `json:"bid_iv"`
	MidIV     float64 `json:"mid_iv"`
	AskIV     float64 `json:"ask_iv"`
	SmvVol    float64 `json:"smv_vol"`
	UpdatedAt string  `json:"updated_at"`
}

// Positions response - matches API exactly
type PositionsResponse struct {
	Positions struct {
		Position []PositionItem `json:"position"`
	} `json:"positions"`
}

type PositionItem struct {
	CostBasis    float64   `json:"cost_basis"`
	DateAcquired time.Time `json:"date_acquired"`
	ID           int       `json:"id"`
	Quantity     float64   `json:"quantity"`     // Negative for short positions
	Symbol       string    `json:"symbol"`       // Will be option symbol for options
}

// Quotes response
type QuotesResponse struct {
	Quotes struct {
		Quote QuoteItem `json:"quote"`
	} `json:"quotes"`
}

type QuoteItem struct {
	Symbol           string  `json:"symbol"`
	Description      string  `json:"description"`
	Exch             string  `json:"exch"`
	Type             string  `json:"type"`
	Last             float64 `json:"last"`
	Change           float64 `json:"change"`
	ChangePercentage float64 `json:"change_percentage"`
	Volume           int64   `json:"volume"`
	AverageVolume    int64   `json:"average_volume"`
	LastVolume       int64   `json:"last_volume"`
	TradeDate        int64   `json:"trade_date"`
	Open             float64 `json:"open"`
	High             float64 `json:"high"`
	Low              float64 `json:"low"`
	Close            float64 `json:"close"`
	PrevClose        float64 `json:"prevclose"`
	Bid              float64 `json:"bid"`
	BidSize          int     `json:"bidsize"`
	BidExch          string  `json:"bidexch"`
	Ask              float64 `json:"ask"`
	AskSize          int     `json:"asksize"`
	AskExch          string  `json:"askexch"`
}

// Expirations response
type ExpirationsResponse struct {
	Expirations struct {
		Date []string `json:"date"`
	} `json:"expirations"`
}

// Account balance response
type BalanceResponse struct {
	Balances struct {
		OptionBuyingPower   float64 `json:"option_buying_power"`
		OptionShortValue    float64 `json:"option_short_value"`
		TotalEquity         float64 `json:"total_equity"`
		AccountValue        float64 `json:"account_value"`
		PendingOrdersCount  int     `json:"pending_orders_count"`
		ClosedPL            float64 `json:"closed_pl"`
		CurrentRequirement  float64 `json:"current_requirement"`
		OptionRequirement   float64 `json:"option_requirement"`
	} `json:"balances"`
}

// Order response
type OrderResponse struct {
	Order struct {
		ID            int     `json:"id"`
		Type          string  `json:"type"`
		Symbol        string  `json:"symbol"`
		Side          string  `json:"side"`
		Quantity      float64 `json:"quantity"`
		Status        string  `json:"status"`
		Duration      string  `json:"duration"`
		Price         float64 `json:"price"`
		AvgFillPrice  float64 `json:"avg_fill_price"`
		ExecQuantity  float64 `json:"exec_quantity"`
		LastFillPrice float64 `json:"last_fill_price"`
		LastFillQuantity float64 `json:"last_fill_quantity"`
		RemainingQuantity float64 `json:"remaining_quantity"`
		CreateDate    string  `json:"create_date"`
		TransactionDate string `json:"transaction_date"`
		Class         string  `json:"class"`
	} `json:"order"`
}

// ============ API Methods ============

func (t *TradierAPI) GetQuote(symbol string) (*QuoteItem, error) {
	endpoint := fmt.Sprintf("%s/markets/quotes?symbols=%s&greeks=false", t.baseURL, symbol)
	
	var response QuotesResponse
	if err := t.makeRequest("GET", endpoint, nil, &response); err != nil {
		return nil, err
	}
	
	return &response.Quotes.Quote, nil
}

func (t *TradierAPI) GetExpirations(symbol string) ([]string, error) {
	endpoint := fmt.Sprintf("%s/markets/options/expirations?symbol=%s&includeAllRoots=true&strikes=false", 
		t.baseURL, symbol)
	
	var response ExpirationsResponse
	if err := t.makeRequest("GET", endpoint, nil, &response); err != nil {
		return nil, err
	}
	
	return response.Expirations.Date, nil
}

func (t *TradierAPI) GetOptionChain(symbol, expiration string, greeks bool) ([]Option, error) {
	endpoint := fmt.Sprintf("%s/markets/options/chains?symbol=%s&expiration=%s&greeks=%t", 
		t.baseURL, symbol, expiration, greeks)
	
	var response OptionChainResponse
	if err := t.makeRequest("GET", endpoint, nil, &response); err != nil {
		return nil, err
	}
	
	return response.Options.Option, nil
}

func (t *TradierAPI) GetPositions() ([]PositionItem, error) {
	endpoint := fmt.Sprintf("%s/accounts/%s/positions", t.baseURL, t.accountID)
	
	var response PositionsResponse
	if err := t.makeRequest("GET", endpoint, nil, &response); err != nil {
		return nil, err
	}
	
	return response.Positions.Position, nil
}

func (t *TradierAPI) GetBalance() (*BalanceResponse, error) {
	endpoint := fmt.Sprintf("%s/accounts/%s/balances", t.baseURL, t.accountID)
	
	var response BalanceResponse
	if err := t.makeRequest("GET", endpoint, nil, &response); err != nil {
		return nil, err
	}
	
	return &response, nil
}

func (t *TradierAPI) PlaceStrangleOrder(
	symbol string,
	putStrike, callStrike float64,
	expiration string,
	quantity int,
	limitCredit float64,
	preview bool,
) (*OrderResponse, error) {
	
	// Convert expiration from YYYY-MM-DD to YYMMDD for option symbol
	expDate, err := time.Parse("2006-01-02", expiration)
	if err != nil {
		return nil, fmt.Errorf("invalid expiration format: %w", err)
	}
	expFormatted := expDate.Format("060102")
	
	// Build option symbols: SYMBOL + YYMMDD + P/C + 8-digit strike
	putSymbol := fmt.Sprintf("%s%sP%08d", symbol, expFormatted, int(putStrike*1000))
	callSymbol := fmt.Sprintf("%s%sC%08d", symbol, expFormatted, int(callStrike*1000))
	
	// Build form data
	params := url.Values{}
	params.Add("class", "multileg")
	params.Add("symbol", symbol)
	params.Add("type", "credit")
	params.Add("duration", "day")
	params.Add("price", fmt.Sprintf("%.2f", limitCredit))
	
	if preview {
		params.Add("preview", "true")
	}
	
	// Leg 0: Sell put
	params.Add("option_symbol[0]", putSymbol)
	params.Add("side[0]", "sell_to_open")
	params.Add("quantity[0]", fmt.Sprintf("%d", quantity))
	
	// Leg 1: Sell call
	params.Add("option_symbol[1]", callSymbol)
	params.Add("side[1]", "sell_to_open")
	params.Add("quantity[1]", fmt.Sprintf("%d", quantity))
	
	endpoint := fmt.Sprintf("%s/accounts/%s/orders", t.baseURL, t.accountID)
	
	var response OrderResponse
	if err := t.makeRequest("POST", endpoint, params, &response); err != nil {
		return nil, err
	}
	
	return &response, nil
}

// Helper method for making HTTP requests
func (t *TradierAPI) makeRequest(method, endpoint string, params url.Values, response interface{}) error {
	var req *http.Request
	var err error
	
	if method == "POST" && params != nil {
		req, err = http.NewRequest(method, endpoint, strings.NewReader(params.Encode()))
		if err != nil {
			return err
		}
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest(method, endpoint, nil)
		if err != nil {
			return err
		}
	}
	
	req.Header.Add("Authorization", "Bearer "+t.apiKey)
	req.Header.Add("Accept", "application/json")
	
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	// Check rate limit headers
	if remaining := resp.Header.Get("X-Ratelimit-Available"); remaining != "" {
		// Could log or track rate limit usage here
	}
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	
	return json.NewDecoder(resp.Body).Decode(response)
}

// ============ Helper Functions ============

// FindStrangleStrikes finds put and call strikes closest to target delta
func FindStrangleStrikes(options []Option, targetDelta float64) (putStrike, callStrike float64, putSymbol, callSymbol string) {
	var bestPut, bestCall *Option
	bestPutDiff := 999.0
	bestCallDiff := 999.0
	
	for i := range options {
		opt := &options[i]
		
		// Skip if no Greeks data
		if opt.Greeks == nil {
			continue
		}
		
		if opt.OptionType == "put" {
			// Put deltas are negative, so we use absolute value
			delta := opt.Greeks.Delta
			if delta < 0 {
				delta = -delta
			}
			
			diff := abs(delta - targetDelta)
			if diff < bestPutDiff {
				bestPutDiff = diff
				bestPut = opt
			}
		} else if opt.OptionType == "call" {
			// Call deltas are positive
			diff := abs(opt.Greeks.Delta - targetDelta)
			if diff < bestCallDiff {
				bestCallDiff = diff
				bestCall = opt
			}
		}
	}
	
	if bestPut != nil {
		putStrike = bestPut.Strike
		putSymbol = bestPut.Symbol
	}
	if bestCall != nil {
		callStrike = bestCall.Strike
		callSymbol = bestCall.Symbol
	}
	
	return
}

// CalculateStrangleCredit calculates expected credit from put and call
func CalculateStrangleCredit(options []Option, putStrike, callStrike float64) float64 {
	var putCredit, callCredit float64
	
	for _, opt := range options {
		if opt.Strike == putStrike && opt.OptionType == "put" {
			// Use mid price between bid and ask
			putCredit = (opt.Bid + opt.Ask) / 2
		}
		if opt.Strike == callStrike && opt.OptionType == "call" {
			// Use mid price between bid and ask
			callCredit = (opt.Bid + opt.Ask) / 2
		}
	}
	
	return putCredit + callCredit
}

// CheckStranglePosition checks if we have an open strangle position
func CheckStranglePosition(positions []PositionItem, symbol string) (hasStrangle bool, putPos, callPos *PositionItem) {
	for i := range positions {
		pos := &positions[i]
		
		// Check if it's an option for our symbol
		if !strings.HasPrefix(pos.Symbol, symbol) {
			continue
		}
		
		// Short positions have negative quantity
		if pos.Quantity >= 0 {
			continue
		}
		
		// Check if it's a put or call
		if strings.Contains(pos.Symbol, "P") {
			putPos = pos
		} else if strings.Contains(pos.Symbol, "C") {
			callPos = pos
		}
	}
	
	hasStrangle = putPos != nil && callPos != nil
	return
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}