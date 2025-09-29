// cleanup_positions - A utility to clean up corrupted position data and orders
// This script will:
// 1. Cancel all pending orders (especially the bogus $0.01 ones)
// 2. Reset positions.json to match broker reality
// 3. Allow bot to start fresh with proper reconciliation
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
)

func main() {
	var (
		configPath = flag.String("config", "config.yaml", "Path to configuration file")
		dryRun     = flag.Bool("dry-run", false, "Show what would be done without making changes")
		cancelOnly = flag.Bool("cancel-only", false, "Only cancel orders, don't clean positions.json")
		resetOnly  = flag.Bool("reset-only", false, "Only reset positions.json, don't cancel orders")
		yes        = flag.Bool("yes", false, "Skip confirmation prompts")
	)
	flag.Parse()

	fmt.Printf("🧹 SCRANTON STRANGLER CLEANUP UTILITY 🧹\n\n")

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Config: %s\n", *configPath)
	fmt.Printf("Broker: %s (sandbox: %t)\n", cfg.Broker.Provider, cfg.Environment.Mode == "paper")
	fmt.Printf("Account: %s\n", cfg.Broker.AccountID)
	fmt.Printf("\n")

	// Create Tradier API client
	sandbox := cfg.Environment.Mode == "paper"
	tradierAPI := broker.NewTradierAPI(cfg.Broker.APIKey, cfg.Broker.AccountID, sandbox)

	// Perform audit first
	fmt.Printf("🔍 AUDITING CURRENT STATE...\n")
	audit, err := tradierAPI.AuditBrokerPositions()
	if err != nil {
		log.Fatalf("Failed to audit broker: %v", err)
	}

	fmt.Printf("Current State:\n")
	fmt.Printf("  - Broker Positions: %d\n", audit.Summary.TotalPositions)
	fmt.Printf("  - Complete Strangles: %d\n", audit.Summary.CompleteStrangles)
	fmt.Printf("  - Open Orders: %d\n", audit.Summary.OpenOrders)
	fmt.Printf("  - Total Cost Basis: $%.2f\n", audit.Summary.TotalCostBasis)
	fmt.Printf("\n")

	// Show problematic orders
	if len(audit.OpenOrders) > 0 {
		fmt.Printf("🚨 PROBLEMATIC ORDERS FOUND:\n")
		for _, order := range audit.OpenOrders {
			fmt.Printf("  Order #%d: %s %s @ $%.2f (%s)\n", 
				order.ID, order.Side, order.Symbol, order.Price, order.Status)
		}
		fmt.Printf("\n")
	}

	// Confirmation
	if !*yes && !*dryRun {
		fmt.Printf("⚠️  This will:\n")
		if !*resetOnly {
			fmt.Printf("  1. Cancel %d open orders\n", len(audit.OpenOrders))
		}
		if !*cancelOnly {
			fmt.Printf("  2. Reset positions.json to match broker reality\n")
			fmt.Printf("  3. Clear any phantom/duplicate position records\n")
		}
		fmt.Printf("\nProceed? (yes/no): ")
		
		var response string
		_, err := fmt.Scanln(&response)
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			return
		}
		if response != "yes" && response != "y" {
			fmt.Println("❌ Cleanup cancelled")
			return
		}
	}

	// PHASE 1: Cancel all orders
	if !*resetOnly {
		fmt.Printf("🗑️  PHASE 1: CANCELLING ORDERS...\n")
		if *dryRun {
			fmt.Printf("DRY RUN: Would cancel %d orders\n", len(audit.OpenOrders))
		} else {
			cancelled := 0
			for _, order := range audit.OpenOrders {
				fmt.Printf("Cancelling order #%d...", order.ID)
				_, err := tradierAPI.CancelOrder(order.ID)
				if err != nil {
					fmt.Printf(" ❌ Failed: %v\n", err)
				} else {
					fmt.Printf(" ✅ Success\n")
					cancelled++
				}
			}
			fmt.Printf("Cancelled %d of %d orders\n\n", cancelled, len(audit.OpenOrders))
		}
	}

	// PHASE 2: Reset positions.json
	if !*cancelOnly {
		fmt.Printf("📄 PHASE 2: RESETTING POSITIONS.JSON...\n")
		if *dryRun {
			fmt.Printf("DRY RUN: Would reset positions.json based on broker data\n")
		} else {
			err := resetPositionsFromBroker(cfg, audit)
			if err != nil {
				log.Fatalf("Failed to reset positions.json: %v", err)
			}
			fmt.Printf("✅ positions.json reset successfully\n\n")
		}
	}

	fmt.Printf("🎉 CLEANUP COMPLETE!\n\n")
	fmt.Printf("Next steps:\n")
	fmt.Printf("  1. Restart the bot: make unraid-restart\n")
	fmt.Printf("  2. Monitor reconciliation in logs: make unraid-logs\n")
	fmt.Printf("  3. Check dashboard for clean state\n")
	fmt.Printf("  4. Verify proper exit orders are created\n")
}

// resetPositionsFromBroker creates a clean positions.json based on actual broker positions
func resetPositionsFromBroker(cfg *config.Config, audit *broker.AuditResult) error {
	// Create storage client
	storageClient, err := storage.NewJSONStorage(cfg.Storage.Path)
	if err != nil {
		return err
	}

	// Clear existing positions and add broker positions
	for _, strangle := range audit.BrokerStrangles {
		if !strangle.IsComplete {
			continue // Skip incomplete strangles for now
		}

		// Create a position record based on the strangle
		position := models.NewPosition(
			generatePositionID(),
			strangle.Symbol,
			extractStrike(strangle.PutPosition),
			extractStrike(strangle.CallPosition),
			parseExpiration(strangle.Expiration),
			int(strangle.TotalQuantity/2), // Divide by 2 since we count both legs
		)

		// Set additional fields
		position.CreditReceived = -strangle.TotalCost // Convert from negative cost to positive credit
		position.EntryDate = time.Now()               // We don't have exact entry date, use now
		position.State = models.StateOpen

		// Add position to storage
		err = storageClient.AddPosition(position)
		if err != nil {
			return fmt.Errorf("failed to add position %s: %w", position.ID, err)
		}
	}

	// Save the updated storage
	return storageClient.Save()
}

// Helper functions
func generatePositionID() string {
	return fmt.Sprintf("cleanup-%d", time.Now().UnixNano())
}

func parseExpiration(brokerExp string) time.Time {
	// Convert "251024" to "2025-10-24"
	if len(brokerExp) != 6 {
		return time.Now().AddDate(0, 1, 0) // Default to 1 month from now
	}
	
	year := "20" + brokerExp[:2]
	month := brokerExp[2:4]
	day := brokerExp[4:6]
	
	exp, err := time.Parse("2006-01-02", fmt.Sprintf("%s-%s-%s", year, month, day))
	if err != nil {
		return time.Now().AddDate(0, 1, 0)
	}
	return exp
}

func extractStrike(pos *broker.PositionItem) float64 {
	if pos == nil {
		return 0
	}
	
	// Extract strike from OSI symbol like "SPY251024P00621000"
	symbol := pos.Symbol
	if len(symbol) < 8 {
		return 0
	}
	
	strikeStr := symbol[len(symbol)-8:]
	strikeInt := 0
	_, err := fmt.Sscanf(strikeStr, "%d", &strikeInt)
	if err != nil {
		return 0
	}
	return float64(strikeInt) / 1000.0 // Convert from thousands
}
