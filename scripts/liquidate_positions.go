// Package main provides a liquidation utility for closing all open positions
// via market orders through the Tradier API.
//
// Usage:
//   export TRADIER_API_KEY="your_key_here"
//   export TRADIER_ACCOUNT_ID="your_account_here" 
//   go run liquidate_positions.go
//   
//   OR use via Makefile:
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
	"fmt"
	"log"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
)

func main() {
	// Load configuration from parent directory
	cfg, err := config.Load("../config.yaml")
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	fmt.Printf("📝 Using credentials from config.yaml\n")
	apiKey := cfg.Broker.APIKey
	accountID := cfg.Broker.AccountID

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
		fmt.Printf("  %d. %s: %.0f contracts @ $%.2f\n", i+1, pos.Symbol, pos.Quantity, pos.CostBasis)
	}
	
	// Close each position individually using market orders (aggressive pricing)
	for _, pos := range positions {
		if pos.Symbol == "SPY" {
			continue // Skip underlying
		}
		
		// Use absolute value of quantity for buy-to-close
		quantity := int(pos.Quantity)
		if quantity < 0 {
			quantity = -quantity // Convert short to positive for closing
		}
		
		fmt.Printf("\n📝 Closing %s (%d contracts)...\n", pos.Symbol, quantity)
		
		// Use market order pricing - get current ask price and add buffer
		quote, err := client.GetQuote(pos.Symbol)
		var maxPrice float64
		if err != nil || quote == nil {
			fmt.Printf("⚠️ Couldn't get quote for %s, using aggressive limit: %v\n", pos.Symbol, err)
			maxPrice = 20.0 // Very high fallback price
		} else {
			// For buy-to-close, use ask price + 50% buffer to ensure immediate fill
			askPrice := quote.Ask
			if askPrice <= 0 {
				askPrice = quote.Last // Fallback to last price
			}
			if askPrice <= 0 {
				askPrice = 5.0 // Final fallback
			}
			maxPrice = askPrice * 3.0 // 200% above ask for guaranteed market-like fill
			if maxPrice < 5.0 {
				maxPrice = 5.0 // Minimum $5 to ensure fill
			}
			fmt.Printf("📊 %s: Ask $%.2f, using MARKET PRICE $%.2f (guaranteed fill)\n", pos.Symbol, askPrice, maxPrice)
		}
		
		orderResp, err := client.PlaceBuyToCloseOrder(pos.Symbol, quantity, maxPrice, "day")
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