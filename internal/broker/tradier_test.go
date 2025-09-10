package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAPIError_Error(t *testing.T) {
	err := &APIError{Status: 429, Body: "too many requests"}
	want := "API error 429: too many requests"
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestNewTradierAPIWithBaseURL_DefaultsAndNormalization(t *testing.T) {
	type args struct {
		apiKey    string
		accountID string
		sandbox   bool
		baseURL   string
	}
	tests := []struct {
		name        string
		args        args
		wantBaseURL string
		wantLimits  RateLimits
	}{
		{
			name:        "sandbox default baseURL and limits",
			args:        args{"k", "acc", true, ""},
			wantBaseURL: "https://sandbox.tradier.com/v1",
			wantLimits:  RateLimits{MarketData: 120, Trading: 120, Standard: 120},
		},
		{
			name:        "production default baseURL and limits",
			args:        args{"k", "acc", false, ""},
			wantBaseURL: "https://api.tradier.com/v1",
			wantLimits:  RateLimits{MarketData: 500, Trading: 500, Standard: 500},
		},
		{
			name:        "custom baseURL preserved and trimmed",
			args:        args{"k", "acc", false, "https://example.test/api/"},
			wantBaseURL: "https://example.test/api",
			wantLimits:  RateLimits{MarketData: 500, Trading: 500, Standard: 500},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewTradierAPIWithBaseURL(tt.args.apiKey, tt.args.accountID, tt.args.sandbox, tt.args.baseURL)
			if api.baseURL != tt.wantBaseURL {
				t.Fatalf("baseURL = %q, want %q", api.baseURL, tt.wantBaseURL)
			}
			if api.rateLimits != tt.wantLimits {
				t.Fatalf("rateLimits = %+v, want %+v", api.rateLimits, tt.wantLimits)
			}
		})
	}
}

func TestNewTradierAPIWithBaseURL_CustomLimitsOverride(t *testing.T) {
	custom := RateLimits{MarketData: 1, Trading: 2, Standard: 3}
	api := NewTradierAPIWithBaseURL("k", "acc", false, "", custom)
	if api.rateLimits != custom {
		t.Fatalf("rateLimits = %+v, want %+v", api.rateLimits, custom)
	}
}

func TestTradierNormalizeDuration(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"day", "day", false},
		{"DAY", "day", false},
		{"  day  ", "day", false},
		{"gtc", "gtc", false},
		{"GTC", "gtc", false},
		{"good-til-cancelled", "gtc", false},
		{"goodtilcancelled", "gtc", false},
		{"pre", "pre", false},
		{"PRE", "pre", false},
		{"  pre  ", "pre", false},
		{"pre-market", "pre", false},
		{"premarket", "pre", false},
		{"extended-hours-pre", "pre", false},
		{"prehours", "pre", false},
		{"post", "post", false},
		{"POST", "post", false},
		{"  post  ", "post", false},
		{"post-market", "post", false},
		{"postmarket", "post", false},
		{"extended-hours-post", "post", false},
		{"posthours", "post", false},
		{"gtd", "", true},
		{"good-til-date", "", true},
		{"", "", true},
		{"week", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := normalizeDuration(tt.in)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeDuration(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func newTestAPIWithServer(handler http.HandlerFunc) (*TradierAPI, *httptest.Server) {
	s := httptest.NewServer(handler)
	api := NewTradierAPIWithBaseURL("test-key", "ACC123", false, s.URL)
	// Use server's client directly to ensure proper transport handling
	api = api.WithHTTPClient(s.Client())
	return api, s
}

func TestMakeRequestCtx_SuccessGET(t *testing.T) {
	type payload struct {
		Foo string `json:"foo"`
	}
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept = %q, want application/json", got)
		}
		w.Header().Set("X-RateLimit-Remaining", "42")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload{Foo: "bar"})
	})
	defer srv.Close()

	var out payload
	if err := api.makeRequest("GET", api.baseURL+"/ok", nil, &out); err != nil {
		t.Fatalf("makeRequest error: %v", err)
	}
	if out.Foo != "bar" {
		t.Fatalf("decoded = %+v, want Foo=bar", out)
	}
}

func TestMakeRequestCtx_SuccessPOST_FormEncoded(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if got := string(body); got != "a=1&b=two" && got != "b=two&a=1" {
			t.Fatalf("body = %q, want form-encoded", got)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer srv.Close()

	var out map[string]any
	if err := api.makeRequest("POST", api.baseURL+"/create", url.Values{"a": []string{"1"}, "b": []string{"two"}}, &out); err != nil {
		t.Fatalf("makeRequest POST error: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("decoded ok=false, want true")
	}
}

func TestMakeRequestCtx_Non2xxReturnsAPIError(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusTeapot)
	})
	defer srv.Close()

	var out map[string]any
	err := api.makeRequest("GET", api.baseURL+"/err", nil, &out)
	if err == nil {
		t.Fatalf("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Status != http.StatusTeapot || apiErr.Body == "" {
		t.Fatalf("APIError = %+v, want status 418 with body", apiErr)
	}
}

func TestGetQuote_SingleAndArrayAndEmpty(t *testing.T) {
	// JSON bodies for single and array responses
	single := `{"quotes":{"quote":{"symbol":"AAPL","description":"Apple","exch":"NMS","type":"stock","askexch":"Q","bidexch":"Q","trade_date":0,"low":0,"average_volume":0,"last_volume":0,"change_percentage":0,"open":0,"high":0,"volume":0,"close":0,"prevclose":0,"bid":10,"bidsize":1,"change":0,"ask":12,"asksize":1,"last":11}}}`
	array := `{"quotes":{"quote":[{"symbol":"AAPL","description":"Apple","exch":"NMS","type":"stock","askexch":"Q","bidexch":"Q","trade_date":0,"low":0,"average_volume":0,"last_volume":0,"change_percentage":0,"open":0,"high":0,"volume":0,"close":0,"prevclose":0,"bid":10,"bidsize":1,"change":0,"ask":12,"asksize":1,"last":11}]}}`
	empty := `{"quotes":{"quote":[]}}`

	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"single", single, false},
		{"array", array, false},
		{"empty", empty, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.RawQuery, "symbols=AAPL") {
					t.Fatalf("missing symbols query: %s", r.URL.RawQuery)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			})
			defer srv.Close()

			q, err := api.GetQuote("AAPL")
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantErr && q.Symbol != "AAPL" {
				t.Fatalf("quote.Symbol = %q, want AAPL", q.Symbol)
			}
		})
	}
}

func TestGetExpirations(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/markets/options/expirations") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"expirations":{"date":["2025-09-19","2025-10-17"]}}`))
	})
	defer srv.Close()

	dates, err := api.GetExpirations("AAPL")
	if err != nil {
		t.Fatalf("GetExpirations error: %v", err)
	}
	if len(dates) != 2 || dates[0] != "2025-09-19" {
		t.Fatalf("dates = %#v", dates)
	}
}

func TestGetExpirationsCtx(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/markets/options/expirations") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"expirations":{"date":["2025-09-19","2025-10-17"]}}`))
	})
	defer srv.Close()

	ctx := context.Background()
	dates, err := api.GetExpirationsCtx(ctx, "AAPL")
	if err != nil {
		t.Fatalf("GetExpirationsCtx error: %v", err)
	}
	if len(dates) != 2 || dates[0] != "2025-09-19" {
		t.Fatalf("dates = %#v", dates)
	}
}

func TestGetOptionChain(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "greeks=true") {
			t.Fatalf("expected greeks=true")
		}
		w.WriteHeader(http.StatusOK)
		// Provide array form
		_, _ = w.Write([]byte(`{"options":{"option":[{"symbol":"AAPL250101P00150000","description":"AAPL Put","option_type":"put","expiration_date":"2025-01-01","underlying":"AAPL","bid":1,"ask":2,"last":0,"bid_size":1,"ask_size":1,"volume":0,"open_interest":0,"expiration_day":1,"strike":150,"greeks":{"updated_at":"", "delta":-0.3,"gamma":0,"theta":0,"vega":0,"rho":0,"phi":0,"bid_iv":0,"mid_iv":0,"ask_iv":0,"smv_vol":0}}]}}`))
	})
	defer srv.Close()

	opts, err := api.GetOptionChain("AAPL", "2025-01-01", true)
	if err != nil {
		t.Fatalf("GetOptionChain error: %v", err)
	}
	if len(opts) != 1 || opts[0].Symbol != "AAPL250101P00150000" || opts[0].Greeks == nil {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestGetPositions(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/accounts/ACC123/positions") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		// singleOrArray should support object form
		_, _ = w.Write([]byte(`{"positions":{"position":{"date_acquired":"2025-01-02T00:00:00Z","symbol":"AAPL250101P00150000","cost_basis":100.0,"id":1,"quantity":-1}}}`))
	})
	defer srv.Close()

	positions, err := api.GetPositions()
	if err != nil {
		t.Fatalf("GetPositions error: %v", err)
	}
	if len(positions) != 1 || positions[0].Symbol == "" || positions[0].Quantity != -1 {
		t.Fatalf("positions = %+v", positions)
	}
}

func TestGetBalance(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/accounts/ACC123/balances") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"balances":{"account_type":"margin","option_short_value":0,"total_equity":2000,"total_cash":1500,"pending_orders_count":0,"close_pl":0,"current_requirement":0,"option_requirement":0,"margin":{"option_buying_power":1000,"stock_buying_power":2000}}}`))
	})
	defer srv.Close()

	bal, err := api.GetBalance()
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if bal.Balances.TotalEquity != 2000 {
		t.Fatalf("TotalEquity = %v, want 2000", bal.Balances.TotalEquity)
	}
}

func TestPlaceStrangleOrder_NormalizesAndBuildsRequest(t *testing.T) {
	// Validate: normalization of duration and multi-leg params, order type/side for credit (sell_to_open)
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.URL.Path; !strings.HasSuffix(got, "/accounts/ACC123/orders") {
			t.Fatalf("path = %s", got)
		}
		body, _ := io.ReadAll(r.Body)
		// Expect duration normalized to "gtc"
		got := string(body)
		for _, expect := range []string{
			"class=multileg",
			"symbol=AAPL",
			"duration=gtc",
			"type=credit",
			"side%5B0%5D=sell_to_open",
			"side%5B1%5D=sell_to_open",
			"quantity%5B0%5D=2",
			"quantity%5B1%5D=2",
		} {
			if !strings.Contains(got, expect) {
				t.Fatalf("request body missing %q; body=%s", expect, got)
			}
		}
		// Quick sanity: presence of OCC-like leg symbols
		if !strings.Contains(got, "option_symbol%5B0%5D=AAPL") || !strings.Contains(got, "option_symbol%5B1%5D=AAPL") {
			t.Fatalf("missing leg symbols: %s", got)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"order":{"create_date":"","type":"credit","symbol":"AAPL","side":"sell_to_open","class":"multileg","status":"ok","duration":"gtc","transaction_date":"","avg_fill_price":0,"exec_quantity":0,"last_fill_price":0,"last_fill_quantity":0,"remaining_quantity":0,"id":123,"price":1.00,"quantity":2}}`))
	})
	defer srv.Close()

	resp, err := api.PlaceStrangleOrder("AAPL", 95.0, 105.0, "2025-01-17", 2, 1.00, false, "gtc", "")
	if err != nil {
		t.Fatalf("PlaceStrangleOrder error: %v", err)
	}
	if resp.Order.ID != 123 || resp.Order.Type != "credit" || resp.Order.Duration != "gtc" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestPlaceStrangleOrder_ExtendedHoursDurations(t *testing.T) {
	// Test cases for extended-hours durations "pre" and "post"
	testCases := []struct {
		name             string
		inputDuration    string
		expectedDuration string
	}{
		{
			name:             "pre-market duration",
			inputDuration:    "pre",
			expectedDuration: "pre",
		},
		{
			name:             "post-market duration",
			inputDuration:    "post",
			expectedDuration: "post",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Fatalf("method = %s, want POST", r.Method)
				}
				if got := r.URL.Path; !strings.HasSuffix(got, "/accounts/ACC123/orders") {
					t.Fatalf("path = %s", got)
				}
				body, _ := io.ReadAll(r.Body)
				got := string(body)
				
				// Verify expected duration is in request
				expectedParam := fmt.Sprintf("duration=%s", tc.expectedDuration)
				if !strings.Contains(got, expectedParam) {
					t.Fatalf("request body missing %q; body=%s", expectedParam, got)
				}
				
				// Verify other required parameters
				for _, expect := range []string{
					"class=multileg",
					"symbol=SPY",
					"type=credit",
					"side%5B0%5D=sell_to_open",
					"side%5B1%5D=sell_to_open",
					"quantity%5B0%5D=1",
					"quantity%5B1%5D=1",
				} {
					if !strings.Contains(got, expect) {
						t.Fatalf("request body missing %q; body=%s", expect, got)
					}
				}
				
				// Verify option symbols are present
				if !strings.Contains(got, "option_symbol%5B0%5D=SPY") || !strings.Contains(got, "option_symbol%5B1%5D=SPY") {
					t.Fatalf("missing leg symbols: %s", got)
				}
				
				w.WriteHeader(http.StatusAccepted)
				response := fmt.Sprintf(`{"order":{"create_date":"","type":"credit","symbol":"SPY","side":"sell_to_open","class":"multileg","status":"ok","duration":"%s","transaction_date":"","avg_fill_price":0,"exec_quantity":0,"last_fill_price":0,"last_fill_quantity":0,"remaining_quantity":0,"id":456,"price":2.50,"quantity":1}}`, tc.expectedDuration)
				_, _ = w.Write([]byte(response))
			})
			defer srv.Close()

			resp, err := api.PlaceStrangleOrder("SPY", 450.0, 470.0, "2025-02-21", 1, 2.50, false, tc.inputDuration, "")
			if err != nil {
				t.Fatalf("PlaceStrangleOrder error: %v", err)
			}
			if resp.Order.ID != 456 {
				t.Fatalf("unexpected order ID: got %d, want 456", resp.Order.ID)
			}
			if resp.Order.Duration != tc.expectedDuration {
				t.Fatalf("unexpected duration: got %s, want %s", resp.Order.Duration, tc.expectedDuration)
			}
			if resp.Order.Type != "credit" {
				t.Fatalf("unexpected order type: got %s, want credit", resp.Order.Type)
			}
		})
	}
}

func TestPlaceStrangleOrder_ValidationErrors(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called for invalid inputs")
	})
	defer srv.Close()

	// invalid duration
	if _, err := api.PlaceStrangleOrder("AAPL", 95, 105, "2025-01-17", 1, 1.0, false, "bad", ""); err == nil {
		t.Fatalf("expected invalid duration error")
	}
	// invalid price
	if _, err := api.placeStrangleOrderInternal("AAPL", 95, 105, "2025-01-17", 1, 0, false, false, "day", ""); err == nil {
		t.Fatalf("expected invalid price error")
	}
	// invalid quantity
	if _, err := api.placeStrangleOrderInternal("AAPL", 95, 105, "2025-01-17", 0, 1.0, false, false, "day", ""); err == nil {
		t.Fatalf("expected invalid quantity error")
	}
	// invalid strikes
	if _, err := api.placeStrangleOrderInternal("AAPL", 110, 105, "2025-01-17", 1, 1.0, false, false, "day", ""); err == nil {
		t.Fatalf("expected invalid strikes error")
	}
	// invalid expiration format
	if _, err := api.placeStrangleOrderInternal("AAPL", 95, 105, "17-01-2025", 1, 1.0, false, false, "day", ""); err == nil {
		t.Fatalf("expected invalid expiration format error")
	}
	// invalid duration post-normalization guard
	if _, err := api.placeStrangleOrderInternal("AAPL", 95, 105, "2025-01-17", 1, 1.0, false, false, "xxx", ""); err == nil {
		t.Fatalf("expected invalid duration error")
	}
}

func TestPlaceStrangleBuyToClose_SetsDebitAndSide(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got := string(body)
		// debit + buy_to_close
		if !strings.Contains(got, "type=debit") || !strings.Contains(got, "side%5B0%5D=buy_to_close") || !strings.Contains(got, "side%5B1%5D=buy_to_close") {
			t.Fatalf("unexpected body: %s", got)
		}
		// fixed duration "day"
		if !strings.Contains(got, "duration=day") {
			t.Fatalf("duration not day: %s", got)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"order":{"id":456,"type":"debit","side":"buy_to_close","class":"multileg","status":"ok","duration":"day"}}`))
	})
	defer srv.Close()

	resp, err := api.PlaceStrangleBuyToClose("AAPL", 95, 105, "2025-01-17", 3, 2.50, "day", "")
	if err != nil {
		t.Fatalf("PlaceStrangleBuyToClose error: %v", err)
	}
	if resp.Order.ID != 456 || resp.Order.Type != "debit" {
		t.Fatalf("unexpected order: %+v", resp)
	}
}

func TestGetOrderStatus_and_GetOrderStatusCtx(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/accounts/ACC123/orders/789") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"order":{"id":789,"status":"ok"}}`))
	})
	defer srv.Close()

	// Without context
	resp, err := api.GetOrderStatus(789)
	if err != nil || resp.Order.ID != 789 {
		t.Fatalf("GetOrderStatus got (%+v,%v)", resp, err)
	}

	// With context
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp2, err := api.GetOrderStatusCtx(ctx, 789)
	if err != nil || resp2.Order.ID != 789 {
		t.Fatalf("GetOrderStatusCtx got (%+v,%v)", resp2, err)
	}
}

func TestPlaceBuyToCloseOrder_ValidatesInputsAndBuildsForm(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		got := string(body)
		for _, expect := range []string{
			"class=option",
			"symbol=AAPL",
			"option_symbol=AAPL250101C00150000",
			"side=buy_to_close",
			"quantity=5",
			"type=limit",
			"duration=day",
			"price=2.75",
		} {
			if !strings.Contains(got, expect) {
				t.Fatalf("missing %q in body: %s", expect, got)
			}
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"order":{"id":9001,"status":"ok"}}`))
	})
	defer srv.Close()

	// Valid
	resp, err := api.PlaceBuyToCloseOrder("AAPL250101C00150000", 5, 2.75, "day")
	if err != nil || resp.Order.ID != 9001 {
		t.Fatalf("PlaceBuyToCloseOrder got (%+v,%v)", resp, err)
	}

	// Invalids
	if _, err := api.PlaceBuyToCloseOrder("AAPL250101C00150000", 5, 0, "day"); err == nil {
		t.Fatalf("expected error: non-positive price")
	}
	if _, err := api.PlaceBuyToCloseOrder("AAPL250101C00150000", 0, 1, "day"); err == nil {
		t.Fatalf("expected error: non-positive quantity")
	}
	if _, err := api.PlaceBuyToCloseOrder("???", 1, 1, "day"); err == nil {
		t.Fatalf("expected error: invalid underlying extraction")
	}
}

func TestPlaceSellToCloseOrder_ValidatesInputsAndBuildsForm(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		got := string(body)
		for _, expect := range []string{
			"class=option",
			"symbol=AAPL",
			"option_symbol=AAPL250101C00150000",
			"side=sell_to_close",
			"quantity=5",
			"type=limit",
			"duration=day",
			"price=2.75",
		} {
			if !strings.Contains(got, expect) {
				t.Fatalf("missing %q in body: %s", expect, got)
			}
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"order":{"id":9002,"status":"ok"}}`))
	})
	defer srv.Close()

	// Valid
	resp, err := api.PlaceSellToCloseOrder("AAPL250101C00150000", 5, 2.75, "day")
	if err != nil || resp.Order.ID != 9002 {
		t.Fatalf("PlaceSellToCloseOrder got (%+v,%v)", resp, err)
	}

	// Invalids
	if _, err := api.PlaceSellToCloseOrder("AAPL250101C00150000", 5, 0, "day"); err == nil {
		t.Fatalf("expected error: non-positive price")
	}
	if _, err := api.PlaceSellToCloseOrder("AAPL250101C00150000", 0, 1, "day"); err == nil {
		t.Fatalf("expected error: non-positive quantity")
	}
	if _, err := api.PlaceSellToCloseOrder("???", 1, 1, "day"); err == nil {
		t.Fatalf("expected error: invalid underlying extraction")
	}
}

func TestPlaceStrangleOTOCO_ReturnsUnsupported(t *testing.T) {
	// The function should return ErrOTOCOUnsupported without touching network.
	api := NewTradierAPI("k", "acc", false)
	if _, err := api.PlaceStrangleOTOCO("", 0, 0, "", 0, 0, 0, false); err == nil {
		t.Fatalf("expected ErrOTOCOUnsupported")
	}
}

func TestFindStrangleStrikes_SelectsClosestByAbsDelta(t *testing.T) {
	opts := []Option{
		// No greeks -> ignored
		{OptionType: "put", Strike: 90},
		// Puts: take abs(delta)
		{OptionType: "put", Strike: 95, Greeks: &Greeks{Delta: -0.28}, Symbol: "PUT95"},
		{OptionType: "put", Strike: 100, Greeks: &Greeks{Delta: -0.35}, Symbol: "PUT100"},
		// Calls: positive delta
		{OptionType: "call", Strike: 105, Greeks: &Greeks{Delta: 0.29}, Symbol: "CALL105"},
		{OptionType: "call", Strike: 110, Greeks: &Greeks{Delta: 0.42}, Symbol: "CALL110"},
	}
	put, call, putSym, callSym := FindStrangleStrikes(opts, 0.30)
	if put != 95 || call != 105 || putSym != "PUT95" || callSym != "CALL105" {
		t.Fatalf("got put=%v call=%v putSym=%q callSym=%q", put, call, putSym, callSym)
	}
}

func TestCalculateStrangleCredit_MidPricesAndMissingStrikes(t *testing.T) {
	opts := []Option{
		{OptionType: "put", Strike: 95, Bid: 1.0, Ask: 1.4},
		{OptionType: "call", Strike: 105, Bid: 1.2, Ask: 1.6},
	}
	credit, err := CalculateStrangleCredit(opts, 95, 105)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mid put = 1.2, mid call = 1.4 => total 2.6
	if math.Abs(credit-2.6) > 1e-10 {
		t.Fatalf("credit = %v, want 2.6", credit)
	}

	_, err2 := CalculateStrangleCredit(opts, 96, 105)
	if err2 == nil {
		t.Fatalf("expected error when put strike missing")
	}
	_, err3 := CalculateStrangleCredit(opts, 95, 106)
	if err3 == nil {
		t.Fatalf("expected error when call strike missing")
	}
}

func TestCalculateStrangleCredit_EpsilonComparisonEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		opts         []Option
		targetPut    float64
		targetCall   float64
		expectCredit float64
		expectError  bool
	}{
		{
			name: "exact match",
			opts: []Option{
				{OptionType: "put", Strike: 95.0, Bid: 1.0, Ask: 1.4},
				{OptionType: "call", Strike: 105.0, Bid: 1.2, Ask: 1.6},
			},
			targetPut:    95.0,
			targetCall:   105.0,
			expectCredit: 2.6, // (1.0+1.4)/2 + (1.2+1.6)/2 = 1.2 + 1.4 = 2.6
			expectError:  false,
		},
		{
			name: "within epsilon 1e-5",
			opts: []Option{
				{OptionType: "put", Strike: 95.00001, Bid: 1.0, Ask: 1.4},
				{OptionType: "call", Strike: 104.99999, Bid: 1.2, Ask: 1.6},
			},
			targetPut:    95.0,
			targetCall:   105.0,
			expectCredit: 2.6,
			expectError:  false,
		},
		{
			name: "at epsilon boundary StrikeMatchEpsilon",
			opts: []Option{
				{OptionType: "put", Strike: 95.0001, Bid: 1.0, Ask: 1.4},
				{OptionType: "call", Strike: 104.9999, Bid: 1.2, Ask: 1.6},
			},
			targetPut:    95.0,
			targetCall:   105.0,
			expectCredit: 2.6,
			expectError:  false,
		},
		{
			name: "beyond epsilon StrikeMatchEpsilon + 0.00001",
			opts: []Option{
				{OptionType: "put", Strike: 95.0 + StrikeMatchEpsilon + 0.00001, Bid: 1.0, Ask: 1.4},
				{OptionType: "call", Strike: 105.0, Bid: 1.2, Ask: 1.6},
			},
			targetPut:    95.0,
			targetCall:   105.0,
			expectCredit: 0,
			expectError:  true,
		},
		{
			name: "rounding mismatch at boundary",
			opts: []Option{
				{OptionType: "put", Strike: 394.995, Bid: 1.0, Ask: 1.4},
				{OptionType: "call", Strike: 404.995, Bid: 1.2, Ask: 1.6},
			},
			targetPut:    395.0,
			targetCall:   405.0,
			expectCredit: 0,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credit, err := CalculateStrangleCredit(tt.opts, tt.targetPut, tt.targetCall)
			if tt.expectError && err == nil {
				t.Fatalf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.expectError && math.Abs(credit-tt.expectCredit) > 1e-10 {
				t.Fatalf("credit = %v, want %v", credit, tt.expectCredit)
			}
		})
	}
}

func TestCheckStranglePosition_FindsShortPutAndCallForUnderlying(t *testing.T) {
	positions := []PositionItem{
		{Symbol: "MSFT250101P00150000", Quantity: -1}, // different underlying
		{Symbol: "AAPL250101P00150000", Quantity: 0},  // not short
		{Symbol: "AAPL250101P00150000", Quantity: -1}, // short put
		{Symbol: "AAPL250101C00160000", Quantity: -2}, // short call
	}
	has, putPos, callPos := CheckStranglePosition(positions, "AAPL")
	if !has || putPos == nil || callPos == nil {
		t.Fatalf("expected strangle, got has=%v put=%v call=%v", has, putPos, callPos)
	}
	if putPos.Symbol != "AAPL250101P00150000" || callPos.Symbol != "AAPL250101C00160000" {
		t.Fatalf("unexpected positions: put=%+v call=%+v", putPos, callPos)
	}
}

func TestExtractUnderlyingFromOSI_BasicAndEdgeCases(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"AAPL250101P00150000", "AAPL"},
		{"SPY240920C00450000", "SPY"},
		{" TSLA250228P00090000", "TSLA"}, // leading space, TrimSpace on result
		{"XYZ250101X00150000", ""},       // not followed by P/C
		{"AAPL250101P001500000", ""},     // too many digits after strike (part of longer run)
		{"AAPL25010P00150000", ""},       // invalid 6-digit sequence
		{"AAPL", ""},                     // too short
		{"FOO012345P00123456", "FOO"},    // 6 digits then P and 8 digits
		// Edge cases with digits/dots in underlying symbol
		{"BRK.B250101P00150000", "BRK.B"}, // dot in symbol
		{"BRK.B240920C00450000", "BRK.B"}, // Berkshire Hathaway class B
		{"A2B250101P00150000", "A2B"},     // digit in symbol
		{"C3.AI250101C00150000", "C3.AI"}, // mixed digits and dots
		{"123456P00123456", ""},           // all digits, no valid underlying
		{"A250101P00150000", "A"},         // single letter underlying
		{"VERYVERYLONGUNDERLYINGSYMBOL250101P00150000", "VERYVERYLONGUNDERLYINGSYMBOL"}, // long underlying
		{"SPY250101P00150000EXTRA", ""}, // extra characters after valid symbol
		{"SPY250101P0015000", ""},       // strike too short (7 digits instead of 8)
		{"SPY250101P001500000", ""},     // strike too long (9 digits)
		{"SPY250101P0015000A", ""},      // non-digit in strike
		{"SPY250101P00150000 ", "SPY"},  // trailing space (should be trimmed)
		{"spy250101p00150000", "spy"},   // lowercase (underlying preserves case)
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := extractUnderlyingFromOSI(tc.in); got != tc.want {
				t.Fatalf("extractUnderlyingFromOSI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTradierOptionTypeFromSymbol(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"AAPL250101P00150000", "put"},
		{"AAPL250101C00150000", "call"},
		{"AAPL250101X00150000", ""},
		{"AAPL250101P0015000", ""}, // not 8 digits at end
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := optionTypeFromSymbol(tc.in); got != tc.want {
				t.Fatalf("optionTypeFromSymbol(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDigitHelpers(t *testing.T) {
	if !isSixDigits("123456") || isSixDigits("12345a") || isSixDigits("1234567") {
		t.Fatalf("isSixDigits failed")
	}
	if !isEightDigits("12345678") || isEightDigits("1234567a") || isEightDigits("123456789") {
		t.Fatalf("isEightDigits failed")
	}
}

// Additional regression: ensure makeRequest returns nil on 200+EOF
func TestMakeRequest_EmptyBodyEOF(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// write no body -> EOF on decode
	})
	defer srv.Close()

	var out struct{}
	if err := api.makeRequest("GET", api.baseURL+"/nobody", nil, &out); err != nil {
		t.Fatalf("unexpected error on EOF: %v", err)
	}
}

// Ensure POST encodes form values deterministically even if map iteration order differs
func TestMakeRequest_PostBodyContainsAllFields(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got := string(b)
		for _, kv := range []string{"x=1", "y=2", "z=hello+world"} {
			if !strings.Contains(got, kv) {
				t.Fatalf("missing %q in body: %s", kv, got)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer srv.Close()

	form := url.Values{"x": []string{"1"}, "y": []string{"2"}, "z": []string{"hello world"}}
	var out map[string]any
	if err := api.makeRequest("POST", api.baseURL+"/post", form, &out); err != nil {
		t.Fatalf("makeRequest POST error: %v", err)
	}
}

// Ensure PlaceStrangleOrder preview flag adds preview=true
func TestPlaceStrangleOrder_PreviewAddsFlag(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "preview=true") {
			t.Fatalf("expected preview=true in body: %s", string(body))
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"order":{"id":321}}`))
	})
	defer srv.Close()

	if _, err := api.placeStrangleOrderInternal("AAPL", 95, 105, "2025-01-17", 1, 1.0, true, false, "day", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Ensure GetQuote constructs correct endpoint query params (greeks=false)
func TestGetQuote_GreeksFalseParam(t *testing.T) {
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "greeks=false") {
			t.Fatalf("expected greeks=false, got query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"quotes":{"quote":{"symbol":"AAPL","description":"","exch":"","type":"","askexch":"","bidexch":"","trade_date":0,"low":0,"average_volume":0,"last_volume":0,"change_percentage":0,"open":0,"high":0,"volume":0,"close":0,"prevclose":0,"bid":0,"bidsize":0,"change":0,"ask":0,"asksize":0,"last":0}}}`))
	})
	defer srv.Close()

	if _, err := api.GetQuote("AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Ensure GetOrderStatusCtx propagates context cancellation (simulate by hanging server and canceling)
func TestGetOrderStatusCtx_ContextCancel(t *testing.T) {
	// Use a server that never responds to force client to respect context; since we can't hook transport timeout,
	// we simulate by canceling before request and ensuring an error is returned from makeRequestCtx.
	api := NewTradierAPIWithBaseURL("k", "ACC", false, "http://127.0.0.1:0") // invalid URL to force error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := api.GetOrderStatusCtx(ctx, 1)
	if err == nil {
		t.Fatalf("expected error due to canceled context")
	}
}

// Utility to pretty-print form bodies in test failure output
func prettyForm(raw string) string {
	dec, _ := url.QueryUnescape(raw)
	return dec
}

// Sanity: ensure placeStrangleOrderInternal builds OCC symbols correctly (YYMMDD and 1/1000 rounding)
func TestPlaceStrangleOrderInternal_OCCSymbolBuild(t *testing.T) {
	var captured string
	api, srv := newTestAPIWithServer(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"order":{"id":777}}`))
	})
	defer srv.Close()

	// 2025-01-02 -> 250102
	if _, err := api.placeStrangleOrderInternal("AAPL", 95.1234, 105.9876, "2025-01-02", 1, 1.00, false, false, "day", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dec := prettyForm(captured)
	if !strings.Contains(dec, "option_symbol[0]=AAPL250102P00095123") {
		t.Fatalf("put symbol not formatted as expected: %s", dec)
	}
	if !strings.Contains(dec, "option_symbol[1]=AAPL250102C00105988") {
		t.Fatalf("call symbol not formatted as expected: %s", dec)
	}
}
