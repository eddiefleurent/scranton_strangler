// Package main provides a liquidation utility for closing all open positions
// via market orders through the Tradier API.
//
// Usage:
//   # Option A: use env vars, no config file required
//   export TRADIER_API_KEY="your_key_here"
//   export TRADIER_ACCOUNT_ID="your_account_here"
//   go run scripts/liquidate_positions.go
//   
//   # Option B: via Makefile:
//   make liquidate
//
//   # Skip confirmation prompt:
//   go run scripts/liquidate_positions.go --yes
//   go run scripts/liquidate_positions.go -y
//
//   # Force liquidation outside market hours:
//   go run scripts/liquidate_positions.go --force
//
// This tool will:
// 1. Fetch all current positions from the broker
// 2. Place market orders for immediate execution
// 3. Report order placement status
//
// Note: In Tradier sandbox, orders may not fill reliably due to platform limitations.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
)

func main() {
	// Command line flags
	cfgPath := flag.String("config", "./config.yaml", "Path to config.yaml")
	yes := flag.Bool("yes", false, "Skip confirmation prompt")
	y := flag.Bool("y", false, "Skip confirmation prompt (shorthand)")
	force := flag.Bool("force", false, "Force liquidation even outside market hours")
	flag.Parse()
	
	// Combine yes flags
	skipConfirm := *yes || *y
	
	var cfg *config.Config
	if *cfgPath != "" {
		if c, err := config.Load(*cfgPath); err == nil {
			cfg = c
		} else if os.Getenv("TRADIER_API_KEY") == "" || os.Getenv("TRADIER_ACCOUNT_ID") == "" {
			log.Fatalf("❌ Failed to load config and env vars missing: %v", err)
		}
	}

	fmt.Printf("📝 Loading credentials (env overrides config)...\n")
	apiKey := ""
	if cfg != nil {
		apiKey = cfg.Broker.APIKey
	}
	if v := os.Getenv("TRADIER_API_KEY"); v != "" {
		apiKey = v
		fmt.Printf("✅ Using TRADIER_API_KEY from environment\n")
	}
	accountID := ""
	if cfg != nil {
		accountID = cfg.Broker.AccountID
	}
	if v := os.Getenv("TRADIER_ACCOUNT_ID"); v != "" {
		accountID = v
		fmt.Printf("✅ Using TRADIER_ACCOUNT_ID from environment\n")
	}

	// Create broker client
	if apiKey == "" || accountID == "" {
		log.Fatalf("❌ Missing Tradier credentials: TRADIER_API_KEY and TRADIER_ACCOUNT_ID must be set via config or env")
	}
	client, err := broker.NewTradierClient(apiKey, accountID, true, false, 0.5)
	if err != nil {
		log.Fatalf("Failed to create Tradier client: %v", err)
	}
	
	// Check market session before proceeding (unless forced)
	if !*force {
		if !checkMarketSession(client) {
			fmt.Println("💡 Use --force flag to override market session check")
			os.Exit(1)
		}
	}

	fmt.Println("💥 LIQUIDATE ALL POSITIONS - MARKET ORDERS 💥")
	// Mask account ID for security
	if n := len(accountID); n > 4 {
		fmt.Printf("🏦 Account: ****%s\n", accountID[n-4:])
	} else {
		fmt.Printf("🏦 Account: %s\n", accountID)
	}
	fmt.Println("⚠️  WARNING: This will close ALL positions using market orders")
	
	if !skipConfirm {
		fmt.Print("\n❓ Are you sure you want to proceed? (yes/no): ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("Failed to read confirmation: %v", err)
		}
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "yes" && response != "y" {
			fmt.Println("❌ Liquidation cancelled")
			os.Exit(0)
		}
	}
	
	fmt.Println("🔒 Proceeding with liquidation via API...")
	
	// Cancel ALL pending orders first
	fmt.Println("🔍 Checking for pending orders to cancel...")
	ctxList, cancelList := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelList()
	ordersResp, err := client.GetOrdersCtx(ctxList)
	if err != nil {
		log.Printf("⚠️  Warning: Could not retrieve orders: %v", err)
	} else if ordersResp == nil || len(ordersResp.Orders.Order) == 0 {
		fmt.Println("✅ No pending orders found")
	} else {
		pendingCount := 0
		cancelledCount := 0
		
		// Define all active order statuses that should be cancelled
		activeStatuses := map[string]struct{}{
			"pending":        {},
			"open":           {},
			"submitted":      {},
			"accepted":       {},
			"partially_filled": {},
			"new":            {},
			"queued":         {},
			"working":        {},
			"pending_cancel": {},
			"replaced":       {},
		}
		
		for _, order := range ordersResp.Orders.Order {
			// Cancel orders that are still active (not filled/canceled/expired/rejected)
			status := strings.ToLower(order.Status)
			if _, isActive := activeStatuses[status]; isActive {
				pendingCount++
				fmt.Printf("📋 Cancelling %s order: %s %s %s (ID: %v)\n",
					status, order.Side, order.Symbol, order.Type, order.ID)
				
				ctxCancel, cancelCancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, cancelErr := client.CancelOrderCtx(ctxCancel, order.ID)
				cancelCancel() // Clean up immediately after each order
				if cancelErr != nil {
					fmt.Printf("❌ Failed to cancel order %v: %v\n", order.ID, cancelErr)
				} else {
					fmt.Printf("✅ Successfully cancelled order %v\n", order.ID)
					cancelledCount++
				}
			}
		}
		
		if pendingCount == 0 {
			fmt.Println("✅ No pending orders found")
		} else {
			fmt.Printf("📊 Cancelled %d of %d pending orders\n", cancelledCount, pendingCount)
			if cancelledCount < pendingCount {
				fmt.Printf("⚠️  %d orders failed to cancel - proceeding with liquidation anyway\n", pendingCount-cancelledCount)
			}
		}
	}
	
	// Get current positions first
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	positions, err := client.GetPositionsCtx(ctx)
	if err != nil {
		log.Fatalf("Failed to get positions: %v", err)
	}
	
	if len(positions) == 0 {
		fmt.Println("✅ No positions to close.")
		return
	}
	fmt.Printf("Found %d positions to close:\n", len(positions))
	for i, pos := range positions {
		fmt.Printf("  %d. %s: %.0f units @ $%.2f\n", i+1, pos.Symbol, math.Abs(pos.Quantity), pos.CostBasis)
	}
	
	// Close each position individually using market orders
	for _, pos := range positions {
		if pos.Symbol == "SPY" {
			continue // Skip underlying
		}
		
		// Filter to only include OSI format option symbols - skip equity positions
		if !isOSIOptionSymbol(pos.Symbol) {
			fmt.Printf("⏭️  Skipping %s: not an OSI option symbol (equity position)\n", pos.Symbol)
			continue
		}
		
		// Determine position direction and appropriate close order type
		absQty := math.Abs(pos.Quantity)
		rounded := int(math.Round(absQty))
		if math.Abs(absQty-float64(rounded)) > 1e-6 {
			fmt.Printf("⏭️  Skipping %s: fractional quantity %.4f not supported by market liquidation; close manually\n", pos.Symbol, absQty)
			continue
		}
		quantity := rounded
		if quantity <= 0 {
			fmt.Printf("⏭️  Skipping %s: computed quantity is 0\n", pos.Symbol)
			continue
		}
		isShort := pos.Quantity < 0
		
		orderType := "buy-to-close MARKET"
		if !isShort {
			orderType = "sell-to-close MARKET"
		}
		
		fmt.Printf("\n📝 Closing %s (%d units) using %s order...\n", pos.Symbol, quantity, orderType)
		
		// Place market order for immediate execution
		var orderResp *broker.OrderResponse
		if isShort {
			// Buy-to-close market order
			fmt.Printf("💰 Using market order for immediate execution\n")
			orderResp, err = client.PlaceBuyToCloseMarketOrder(pos.Symbol, quantity, string(broker.DurationDay), "emergency-liquidation")
		} else {
			// Sell-to-close market order
			fmt.Printf("💰 Using market order for immediate execution\n")
			orderResp, err = client.PlaceSellToCloseMarketOrder(pos.Symbol, quantity, string(broker.DurationDay), "emergency-liquidation")
		}
		if err != nil {
			fmt.Printf("❌ Failed to close %s: %v\n", pos.Symbol, err)
			continue
		}
		
		if orderResp != nil && orderResp.Order.ID != 0 {
			fmt.Printf("✅ Close order placed: Order ID %d\n", orderResp.Order.ID)
		} else {
			fmt.Printf("⚠️ Order response received but no order ID\n")
		}
	}
	
	fmt.Println("\n🎯 All close orders submitted!")
	fmt.Println("⏳ Orders may take a few minutes to fill in sandbox environment")
	fmt.Println("🔍 Monitor with: make test-api")
}

// checkMarketSession verifies if market is open using Tradier API
func checkMarketSession(client broker.Broker) bool {
	fmt.Println("🕒 Checking market session...")
	
	// Note: GetMarketClock doesn't have a Ctx variant in the interface,
	// but we can add a timeout wrapper if needed in the future
	clock, err := client.GetMarketClock(false)
	if err != nil {
		fmt.Printf("⚠️  Could not get market status: %v\n", err)
		fmt.Println("💡 Falling back to basic time check...")
		return isWithinETHours()
	}
	
	if clock == nil {
		fmt.Println("⚠️  No market clock data received")
		return isWithinETHours()
	}
	
	state := clock.Clock.State
	fmt.Printf("📊 Market status: %s\n", state)
	
	if state == "open" {
		return true
	}
	
	// Market is closed - warn about DAY order behavior
	fmt.Printf("🔴 Market is currently %s\n", state)
	fmt.Println("⚠️  DAY market orders may queue or be rejected outside RTH")
	return false
}

// isWithinETHours provides basic ET trading hours check (9:30 AM - 4:00 PM ET, Mon-Fri)
func isWithinETHours() bool {
	now := time.Now()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		// Fallback to EST approximation
		loc = time.FixedZone("EST", -5*60*60)
	}
	etTime := now.In(loc)
	
	// Check if it's a weekday
	weekday := etTime.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		fmt.Println("📅 Weekend - market closed")
		return false
	}
	
	// Check trading hours (9:30 AM - 4:00 PM ET)
	hour := etTime.Hour()
	minute := etTime.Minute()
	currentMinutes := hour*60 + minute
	
	openMinutes := 9*60 + 30   // 9:30 AM
	closeMinutes := 16*60      // 4:00 PM
	
	if currentMinutes >= openMinutes && currentMinutes < closeMinutes {
		fmt.Printf("🟢 Within regular trading hours (%02d:%02d ET)\n", hour, minute)
		return true
	}
	
	fmt.Printf("🔴 Outside trading hours (%02d:%02d ET)\n", hour, minute)
	fmt.Println("📋 Regular hours: 9:30 AM - 4:00 PM ET, Monday-Friday")
	return false
}

// isOSIOptionSymbol checks if a symbol follows OSI (Options Symbology Initiative) format
// OSI format: TICKER + YYMMDD + P/C + 8-digit strike price (e.g., SPY241220P00450000)
// Returns true for option symbols, false for equity symbols like "SPY", "AAPL"
func isOSIOptionSymbol(symbol string) bool {
	trimmedSymbol := strings.TrimSpace(symbol)
	
	// OSI symbols must be at least 15 characters (3-char ticker + 6-digit date + P/C + 8-digit strike)
	if len(trimmedSymbol) < 15 {
		return false
	}
	
	// Check if it ends with exactly 8 digits (strike price)
	if len(trimmedSymbol) < 8 {
		return false
	}
	strikeStr := trimmedSymbol[len(trimmedSymbol)-8:]
	for _, ch := range strikeStr {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	
	// Check if there's a P or C before the strike (option type)
	if len(trimmedSymbol) < 9 {
		return false
	}
	optionType := trimmedSymbol[len(trimmedSymbol)-9]
	if optionType != 'P' && optionType != 'C' {
		return false
	}
	
	// Check if there are at least 6 digits before the option type (date)
	if len(trimmedSymbol) < 15 {
		return false
	}
	dateStr := trimmedSymbol[len(trimmedSymbol)-15 : len(trimmedSymbol)-9]
	for _, ch := range dateStr {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	
	return true
}