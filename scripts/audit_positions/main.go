// audit_positions - A utility to audit broker positions vs local storage
// This script helps identify discrepancies between what the bot thinks it has
// and what's actually in the broker account.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
)

// maskAccountID masks all but the last 4 characters of an account ID to prevent PII exposure
func maskAccountID(id string) string {
	if len(id) > 4 {
		return strings.Repeat("*", len(id)-4) + id[len(id)-4:]
	}
	return id
}

func main() {
	var (
		configPath = flag.String("config", "config.yaml", "Path to configuration file")
		jsonOutput = flag.Bool("json", false, "Output results as JSON")
		verbose    = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if *verbose {
		fmt.Printf("Using config: %s\n", *configPath)
		fmt.Printf("Broker: %s (sandbox: %t)\n", cfg.Broker.Provider, cfg.Environment.Mode == "paper")
		fmt.Printf("Account ID: %s\n", maskAccountID(cfg.Broker.AccountID))
		fmt.Printf("\n")
	}

	// Create Tradier API client
	sandbox := cfg.Environment.Mode == "paper"
	tradierAPI := broker.NewTradierAPI(cfg.Broker.APIKey, cfg.Broker.AccountID, sandbox)

	// Perform audit
	fmt.Printf("Auditing broker positions and orders...\n")
	audit, err := tradierAPI.AuditBrokerPositions()
	if err != nil {
		log.Fatalf("Failed to audit broker positions: %v", err)
	}

	// Output results
	if *jsonOutput {
		output, err := json.MarshalIndent(audit, "", "  ")
		if err != nil {
			log.Fatalf("Failed to marshal JSON: %v", err)
		}
		fmt.Println(string(output))
	} else {
		audit.PrintAuditReport()
	}

	// Additional analysis
	if !*jsonOutput {
		fmt.Printf("=== ANALYSIS ===\n")
		
		// Check for potential issues
		issues := analyzeAuditResults(audit)
		if len(issues) > 0 {
			fmt.Printf("POTENTIAL ISSUES FOUND:\n")
			for i, issue := range issues {
				fmt.Printf("  %d. %s\n", i+1, issue)
			}
		} else {
			fmt.Printf("No obvious issues detected.\n")
		}
		
		fmt.Printf("\n")
		fmt.Printf("Next steps:\n")
		fmt.Printf("  1. Compare this with local positions.json\n")
		fmt.Printf("  2. Check if P&L calculations match broker values\n")
		fmt.Printf("  3. Verify exit orders are properly placed\n")
		fmt.Printf("  4. Reconcile any missing or extra positions\n")
	}
}

// analyzeAuditResults performs basic analysis to identify potential issues
func analyzeAuditResults(audit *broker.AuditResult) []string {
	var issues []string

	// Nil-safety checks
	if audit == nil {
		return issues
	}

	// Check for incomplete strangles
	incompleteStrangles := audit.Summary.TotalStrangles - audit.Summary.CompleteStrangles
	if incompleteStrangles > 0 {
		issues = append(issues, fmt.Sprintf("%d incomplete strangle(s) - missing put or call leg", incompleteStrangles))
	}

	// Check for stale open orders (created more than a day ago)
	// This would require parsing the CreateDate, but for now just check count
	if audit.Summary.OpenOrders > 10 {
		issues = append(issues, fmt.Sprintf("High number of open orders (%d) - may include stale orders", audit.Summary.OpenOrders))
	}

	// Check for negative cost basis (should be negative for short positions)
	if audit.Summary.TotalCostBasis > 0 {
		issues = append(issues, "Positive total cost basis - unusual for short strangle strategy")
	}

	// Check if we have positions but no open orders (might need exit orders)
	if audit.Summary.TotalPositions > 0 && audit.Summary.OpenOrders == 0 {
		issues = append(issues, "Have positions but no open orders - exit orders may be missing")
	}

	return issues
}
