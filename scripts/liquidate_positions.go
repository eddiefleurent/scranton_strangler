// Package main provides a liquidation utility for closing all open positions
// via market orders through the Tradier API.
//
// Usage:
//   # Option A: use env vars, no config required
//   export TRADIER_API_KEY="your_key_here"
//   export TRADIER_ACCOUNT_ID="your_account_here"
//   go run scripts/liquidate_positions.go -config=./config.yaml
//   
//   # Option B: use via Makefile:
//   make liquidate
//
// This tool will:
// 1. Fetch all current positions from the broker
// 2. Place aggressive buy-to-close orders at market prices
// 3. Report order placement status
//
// Note: In Tradier sandbox, orders may not fill reliably due to platform limitations.
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
)

func main() {
	// Load configuration
	cfgPath := flag.String("config", "./config.yaml", "Path to config.yaml")
	flag.Parse()
	
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
	client, err := broker.NewTradierClient(apiKey, accountID, true, false, 0.5)
	if err != nil {
		log.Fatalf("Failed to create Tradier client: %v", err)
	}

	fmt.Println("💥 LIQUIDATE ALL POSITIONS - MARKET ORDERS ONLY 💥")
	fmt.Println("⚠️  WARNING: This will close ALL positions at MARKET PRICES")
	fmt.Println("🔒 Proceeding with liquidation via API...")
	
	// Get current positions first
	positions, err := client.GetPositions()
	if err != nil {
		log.Fatalf("Failed to get positions: %v", err)
	}
	
	fmt.Printf("Found %d positions to close:\n", len(positions))
	for i, pos := range positions {
		fmt.Printf("  %d. %s: %.0f contracts @ $%.2f\n", i+1, pos.Symbol, math.Abs(pos.Quantity), pos.CostBasis)
	}
	
	// Close each position individually using market orders (aggressive pricing)
	for _, pos := range positions {
		if pos.Symbol == "SPY" {
			continue // Skip underlying
		}
		
		// Determine position direction and appropriate close order type
		quantity := int(math.Abs(math.Round(pos.Quantity)))
		isShort := pos.Quantity < 0
		
		orderType := "buy-to-close MARKET"
		if !isShort {
			orderType = "sell-to-close MARKET"
		}
		
		fmt.Printf("\n📝 Closing %s (%d contracts) using %s order...\n", pos.Symbol, quantity, orderType)
		fmt.Printf("💥 MARKET ORDER: Will execute at current market price for immediate fill\n")
		
		// Place the appropriate market order type based on position direction
		var orderResp *broker.OrderResponse
		if isShort {
			orderResp, err = client.PlaceBuyToCloseMarketOrder(pos.Symbol, quantity, "day", "emergency-liquidation")
		} else {
			orderResp, err = client.PlaceSellToCloseMarketOrder(pos.Symbol, quantity, "day", "emergency-liquidation")
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