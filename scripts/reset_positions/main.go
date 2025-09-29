// reset_positions - Reset positions.json to match broker reality
// This creates a clean positions.json file based on actual broker positions
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	outputPath := flag.String("output", "data/positions.json", "Output path for positions.json")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create Tradier API client
	sandbox := cfg.Environment.Mode == "paper"
	tradierAPI := broker.NewTradierAPI(cfg.Broker.APIKey, cfg.Broker.AccountID, sandbox)

	// Get broker positions
	fmt.Printf("🔍 Getting broker positions...\n")
	audit, err := tradierAPI.AuditBrokerPositions()
	if err != nil {
		log.Fatalf("Failed to audit broker: %v", err)
	}

	fmt.Printf("Found:\n")
	fmt.Printf("  - %d broker positions\n", audit.Summary.TotalPositions)
	fmt.Printf("  - %d complete strangles\n", audit.Summary.CompleteStrangles)
	fmt.Printf("  - Total cost basis: $%.2f\n", audit.Summary.TotalCostBasis)

	// Create clean positions.json structure
	cleanData := map[string]interface{}{
		"last_updated":       time.Now().Format(time.RFC3339),
		"current_positions":  []interface{}{},
		"daily_pnl":         map[string]float64{},
		"statistics": map[string]interface{}{
			"total_trades":           0,
			"winning_trades":         0,
			"losing_trades":          0,
			"break_even_trades":      0,
			"win_rate":              0.0,
			"total_pnl":             0.0,
			"average_win":           0.0,
			"average_loss":          0.0,
			"max_single_trade_loss": 0.0,
			"current_streak":        0,
		},
		"history":     []interface{}{},
		"iv_readings": []interface{}{},
	}

	// Convert broker strangles to position records
	positions := []interface{}{}
	for _, strangle := range audit.BrokerStrangles {
		if !strangle.IsComplete {
			continue
		}

		position := map[string]interface{}{
			"id":               fmt.Sprintf("cleanup-%d", time.Now().UnixNano()),
			"symbol":           strangle.Symbol,
			"state":            "open",
			"adjustments":      []interface{}{},
			"expiration":       parseExpiration(strangle.Expiration),
			"entry_date":       time.Now().Format(time.RFC3339),
			"exit_date":        "0001-01-01T00:00:00Z",
			"credit_received":  -strangle.TotalCost, // Convert negative cost to positive credit
			"entry_limit_price": 0,
			"entry_iv":         0,
			"entry_spot":       0,
			"current_pnl":      0,
			"call_strike":      extractStrike(strangle.CallPosition),
			"put_strike":       extractStrike(strangle.PutPosition),
			"quantity":         int(strangle.TotalQuantity / 2), // Divide by 2 for pair
		}
		positions = append(positions, position)
	}

	cleanData["current_positions"] = positions

	// Write to file
	fmt.Printf("\n📄 Writing clean positions.json...\n")
	file, err := os.Create(*outputPath)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cleanData); err != nil {
		log.Fatalf("Failed to encode JSON: %v", err)
	}

	fmt.Printf("✅ Clean positions.json written to: %s\n", *outputPath)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Backup original: cp data/positions.json data/positions.json.backup\n")
	fmt.Printf("  2. Replace with clean version\n")
	fmt.Printf("  3. Restart bot to reconcile\n")
}

func parseExpiration(brokerExp string) string {
	// Convert "251024" to "2025-10-24T00:00:00Z"
	if len(brokerExp) != 6 {
		return time.Now().AddDate(0, 1, 0).Format(time.RFC3339)
	}
	
	year := "20" + brokerExp[:2]
	month := brokerExp[2:4]
	day := brokerExp[4:6]
	
	exp, err := time.Parse("2006-01-02", fmt.Sprintf("%s-%s-%s", year, month, day))
	if err != nil {
		return time.Now().AddDate(0, 1, 0).Format(time.RFC3339)
	}
	return exp.Format(time.RFC3339)
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
	return float64(strikeInt) / 1000.0
}
