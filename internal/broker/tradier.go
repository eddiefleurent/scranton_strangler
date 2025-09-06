// Package broker provides trading API clients for executing options trades.
// It includes the Tradier API client implementation for SPY short strangle strategies.
package broker

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Option type constants
const (
	optionTypePut  = "put"
	optionTypeCall = "call"
)

// TradierAPI - Accurate implementation based on actual API docs
type TradierAPI struct {
	client     *http.Client
	apiKey     string
	baseURL    string
	accountID  string
	rateLimits RateLimits
	sandbox    bool
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

// Handle single-object vs array responses from Tradier
type singleOrArray[T any] []T

func (s *singleOrArray[T]) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		return json.Unmarshal(b, (*[]T)(s))
	}
	var one T
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*s = append(*s, one)
	return nil
}

// Option chain response - matches API exactly
type OptionChainResponse struct {
	Options struct {
		Option singleOrArray[Option] `json:"option"`
	} `json:"options"`
}

type Option struct {
	Greeks         *Greeks `json:"greeks,omitempty"`
	Symbol         string  `json:"symbol"`
	Description    string  `json:"description"`
	OptionType     string  `json:"option_type"`
	ExpirationDate string  `json:"expiration_date"`
	Underlying     string  `json:"underlying"`
	Bid            float64 `json:"bid"`
	Ask            float64 `json:"ask"`
	Last           float64 `json:"last"`
	BidSize        int     `json:"bid_size"`
	AskSize        int     `json:"ask_size"`
	Volume         int64   `json:"volume"`
	OpenInterest   int64   `json:"open_interest"`
	ExpirationDay  int     `json:"expiration_day"`
	Strike         float64 `json:"strike"`
}

type Greeks struct {
	UpdatedAt string  `json:"updated_at"`
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
}

// Positions response - matches API exactly
type PositionsResponse struct {
	Positions struct {
		Position singleOrArray[PositionItem] `json:"position"`
	} `json:"positions"`
}

type PositionItem struct {
	DateAcquired time.Time `json:"date_acquired"`
	Symbol       string    `json:"symbol"`
	CostBasis    float64   `json:"cost_basis"`
	ID           int       `json:"id"`
	Quantity     float64   `json:"quantity"`
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
	AskExch          string  `json:"askexch"`
	BidExch          string  `json:"bidexch"`
	TradeDate        int64   `json:"trade_date"`
	Low              float64 `json:"low"`
	AverageVolume    int64   `json:"average_volume"`
	LastVolume       int64   `json:"last_volume"`
	ChangePercentage float64 `json:"change_percentage"`
	Open             float64 `json:"open"`
	High             float64 `json:"high"`
	Volume           int64   `json:"volume"`
	Close            float64 `json:"close"`
	PrevClose        float64 `json:"prevclose"`
	Bid              float64 `json:"bid"`
	BidSize          int     `json:"bidsize"`
	Change           float64 `json:"change"`
	Ask              float64 `json:"ask"`
	AskSize          int     `json:"asksize"`
	Last             float64 `json:"last"`
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
		OptionBuyingPower  float64 `json:"option_buying_power"`
		OptionShortValue   float64 `json:"option_short_value"`
		TotalEquity        float64 `json:"total_equity"`
		AccountValue       float64 `json:"account_value"`
		PendingOrdersCount int     `json:"pending_orders_count"`
		ClosedPL           float64 `json:"closed_pl"`
		CurrentRequirement float64 `json:"current_requirement"`
		OptionRequirement  float64 `json:"option_requirement"`
	} `json:"balances"`
}

// Order response
type OrderResponse struct {
	Order struct {
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
	limitPrice float64,
	preview bool,
) (*OrderResponse, error) {
	return t.placeStrangleOrderInternal(symbol, putStrike, callStrike, expiration, quantity, limitPrice, preview, false)
}

func (t *TradierAPI) placeStrangleOrderInternal(
	symbol string,
	putStrike, callStrike float64,
	expiration string,
	quantity int,
	limitPrice float64,
	preview bool,
	buyToClose bool,
) (*OrderResponse, error) {
	// Convert expiration from YYYY-MM-DD to YYMMDD for option symbol
	expDate, err := time.Parse("2006-01-02", expiration)
	if err != nil {
		return nil, fmt.Errorf("invalid expiration format: %w", err)
	}
	expFormatted := expDate.Format("060102")

	// Build option symbols: SYMBOL + YYMMDD + P/C + 8-digit strike
	// Use rounded 1/1000th dollars to build OCC strike field
	putSymbol := fmt.Sprintf("%s%sP%08d", symbol, expFormatted, int(math.Round(putStrike*1000)))
	callSymbol := fmt.Sprintf("%s%sC%08d", symbol, expFormatted, int(math.Round(callStrike*1000)))

	// Build form data
	params := url.Values{}
	params.Add("class", "multileg")
	params.Add("symbol", symbol)
	params.Add("duration", "day")
	params.Add("price", fmt.Sprintf("%.2f", limitPrice))

	// Determine order type and side based on buyToClose flag
	var orderType, side string
	if buyToClose {
		orderType = "debit"
		side = "buy_to_close"
	} else {
		orderType = "credit"
		side = "sell_to_open"
	}
	params.Add("type", orderType)

	if preview {
		params.Add("preview", "true")
	}

	// Leg 0: Put option
	params.Add("option_symbol[0]", putSymbol)
	params.Add("side[0]", side)
	params.Add("quantity[0]", fmt.Sprintf("%d", quantity))

	// Leg 1: Call option
	params.Add("option_symbol[1]", callSymbol)
	params.Add("side[1]", side)
	params.Add("quantity[1]", fmt.Sprintf("%d", quantity))

	endpoint := fmt.Sprintf("%s/accounts/%s/orders", t.baseURL, t.accountID)

	var response OrderResponse
	if err := t.makeRequest("POST", endpoint, params, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// PlaceStrangleBuyToClose places a buy-to-close order for a strangle position
func (t *TradierAPI) PlaceStrangleBuyToClose(
	symbol string,
	putStrike, callStrike float64,
	expiration string,
	quantity int,
	maxDebit float64,
) (*OrderResponse, error) {
	return t.placeStrangleOrderInternal(symbol, putStrike, callStrike, expiration, quantity, maxDebit, false, true)
}

// GetOrderStatus retrieves the status of an existing order by ID
func (t *TradierAPI) GetOrderStatus(orderID int) (*OrderResponse, error) {
	endpoint := fmt.Sprintf("%s/accounts/%s/orders/%d", t.baseURL, t.accountID, orderID)
	var response OrderResponse
	if err := t.makeRequest("GET", endpoint, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (t *TradierAPI) PlaceBuyToCloseOrder(optionSymbol string, quantity int, maxPrice float64) (*OrderResponse, error) {
	// Extract underlying symbol from option OCC/OSI code
	symbol := extractUnderlyingFromOSI(optionSymbol)
	if symbol == "" {
		return nil, fmt.Errorf("failed to extract underlying symbol from option symbol: %s", optionSymbol)
	}

	params := url.Values{}
	params.Add("class", "option")
	params.Add("symbol", symbol) // Required underlying symbol
	params.Add("option_symbol", optionSymbol)
	params.Add("side", "buy_to_close")
	params.Add("quantity", fmt.Sprintf("%d", quantity))
	params.Add("type", "limit")
	params.Add("duration", "day")
	params.Add("price", fmt.Sprintf("%.2f", maxPrice))

	endpoint := fmt.Sprintf("%s/accounts/%s/orders", t.baseURL, t.accountID)

	var response OrderResponse
	if err := t.makeRequest("POST", endpoint, params, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (t *TradierAPI) PlaceStrangleOTOCO(
	symbol string,
	putStrike, callStrike float64,
	expiration string,
	quantity int,
	limitCredit float64,
	profitTarget float64, // percentage as decimal (0.5 for 50%)
	preview bool,
) (*OrderResponse, error) {
	// OTOCO orders in Tradier API do not support multi-leg orders like strangles
	// The API documentation specifies that OTOCO is for single-leg orders only
	return nil, fmt.Errorf("OTOCO orders do not support multi-leg strangle orders in Tradier API - use separate orders for opening and closing positions, or implement a polling mechanism to monitor fills and submit individual close orders")
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
		req, err = http.NewRequest(method, endpoint, http.NoBody)
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log error but don't fail the operation
		}
	}()

	// Check rate limit headers
	if remaining := resp.Header.Get("X-Ratelimit-Available"); remaining != "" {
		// Could log or track rate limit usage here
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("API error %d: failed to read error body", resp.StatusCode)
		}
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

		if opt.OptionType == optionTypePut {
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
		} else if opt.OptionType == optionTypeCall {
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

	return putStrike, callStrike, putSymbol, callSymbol
}

// CalculateStrangleCredit calculates expected credit from put and call
func CalculateStrangleCredit(options []Option, putStrike, callStrike float64) float64 {
	var putCredit, callCredit float64

	for _, opt := range options {
		if opt.Strike == putStrike && opt.OptionType == optionTypePut {
			// Use mid price between bid and ask
			putCredit = (opt.Bid + opt.Ask) / 2
		}
		if opt.Strike == callStrike && opt.OptionType == optionTypeCall {
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

		switch optionTypeFromSymbol(pos.Symbol) {
		case "put":
			putPos = pos
		case "call":
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

// extractUnderlyingFromOSI extracts the underlying symbol from an OSI-formatted option symbol
// e.g., "SPY241220P00450000" -> "SPY"
func extractUnderlyingFromOSI(s string) string {
	// OSI format: UNDERLYING + YYMMDD + P/C + 8-digit strike
	// We need to find the start of the 6-digit expiration date
	if len(s) < 16 { // minimum length for a valid option symbol
		return ""
	}

	// Look for the first 6-digit sequence (expiration date) with proper validation
	for i := 0; i <= len(s)-15; i++ { // need at least 15 chars after start for YYMMDD + P/C + 8 digits
		if isSixDigits(s[i:i+6]) {
			// Check that the 6-digit sequence is not part of a longer numeric run
			if i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
				continue // previous char is digit, skip
			}

			// Check that 6-digit expiration is followed by P/C and exactly 8 digits
			if i+15 > len(s) {
				continue // not enough chars remaining
			}

			expirationEnd := i + 6
			typeChar := s[expirationEnd]
			if typeChar != 'P' && typeChar != 'C' {
				continue // not followed by P or C
			}

			strikeStart := expirationEnd + 1
			if !isEightDigits(s[strikeStart : strikeStart+8]) {
				continue // not followed by exactly 8 digits
			}

			// Check that the strike is not part of a longer numeric run
			strikeEnd := strikeStart + 8
			if strikeEnd < len(s) && s[strikeEnd] >= '0' && s[strikeEnd] <= '9' {
				continue // next char is digit, skip
			}

			// All conditions met, return underlying
			return strings.TrimSpace(s[:i])
		}
	}

	return ""
}

// optionTypeFromSymbol returns "put" | "call" | "" from OSI-like symbols, e.g. SPY241220P00450000
func optionTypeFromSymbol(s string) string {
	// Find the type char immediately before the 8-digit strike suffix.
	// E.g., ...P######## or ...C########
	if len(s) < 9 {
		return ""
	}
	// Walk backward to locate the 8 trailing digits
	i := len(s) - 1
	digits := 0
	for i >= 0 && digits < 8 {
		if s[i] < '0' || s[i] > '9' {
			return ""
		}
		i--
		digits++
	}
	if i < 0 {
		return ""
	}
	// The char at i should be 'P' or 'C'
	switch s[i] {
	case 'P':
		return "put"
	case 'C':
		return "call"
	default:
		return ""
	}
}

// isSixDigits checks if a string consists of exactly 6 digits
func isSixDigits(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isEightDigits checks if a string consists of exactly 8 digits
func isEightDigits(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
