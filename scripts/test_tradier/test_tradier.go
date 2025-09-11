// Package main provides a test utility for Tradier API connection testing.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/util"
)

var optionSymbolRegex = regexp.MustCompile(`^[A-Z]{1,6}\d{6}[CP]\d{8}$`)

func main() {
	var sandbox bool
	flag.BoolVar(&sandbox, "sandbox", true, "Use Tradier sandbox endpoints (default: true)")
	flag.Parse()

	fmt.Println("=== Tradier API Complete Test Suite ===")
	fmt.Println()

	// Check for API credentials from environment or config
	apiKey := os.Getenv("TRADIER_API_KEY")
	accountID := os.Getenv("TRADIER_ACCOUNT_ID")

	// If not set via environment, try to read from config.yaml
	if apiKey == "" || accountID == "" {
		var err error
		apiKey, accountID, err = readConfigCredentials()
		if err != nil {
			fmt.Println("❌ Failed to read credentials from config.yaml")
			fmt.Println("Error:", err)
			fmt.Println("\nSetup Instructions:")
			fmt.Println("1. Update config.yaml with your sandbox API key and account ID")
			fmt.Println("2. Or export them as environment variables:")
			fmt.Println("   export TRADIER_API_KEY='your_token_here'")
			fmt.Println("   export TRADIER_ACCOUNT_ID='your_account_id'")
			os.Exit(1)
		}
	}

	if apiKey == "" {
		fmt.Println("❌ TRADIER_API_KEY not set")
		fmt.Println("Please update config.yaml or set TRADIER_API_KEY environment variable")
		os.Exit(1)
	}

	if accountID == "" {
		fmt.Println("⚠️  TRADIER_ACCOUNT_ID not set, some tests will be skipped")
		fmt.Println("Please update config.yaml or set TRADIER_ACCOUNT_ID environment variable")
	}

	// Initialize client
	client, err := broker.NewTradierClient(apiKey, accountID, sandbox, true /* useOTOCO */, 0.5 /* profitTarget */)
	if err != nil {
		log.Fatalf("Failed to create Tradier client: %v", err)
	}
	if sandbox {
		fmt.Printf("✓ Initialized Tradier client (Sandbox mode)\n")
	} else {
		fmt.Printf("✓ Initialized Tradier client (Live mode)\n")
	}
	fmt.Printf("  API Key: %s\n", maskAPIKey(apiKey))
	if accountID != "" {
		fmt.Printf("  Account: %s\n", accountID)
	}
	fmt.Println()

	// Test 1: Get SPY Quote
	fmt.Println("Test 1: Get SPY Quote")
	fmt.Println("=" + strings.Repeat("=", 40))

	quote, err := client.GetQuote("SPY")
	if err != nil {
		fmt.Printf("❌ Error: %v\n\n", err)
	} else {
		fmt.Printf("✓ SPY Quote Retrieved:\n")
		fmt.Printf("  Last: $%.2f\n", quote.Last)
		fmt.Printf("  Bid:  $%.2f (size: %d)\n", quote.Bid, quote.BidSize)
		fmt.Printf("  Ask:  $%.2f (size: %d)\n", quote.Ask, quote.AskSize)
		fmt.Printf("  Volume: %s\n", formatNumber(quote.Volume))
		fmt.Printf("  Change: $%.2f (%.2f%%)\n\n", quote.Change, quote.ChangePercentage)
	}

	// Test 2: Get Option Expirations
	fmt.Println("Test 2: Get SPY Option Expirations")
	fmt.Println("=" + strings.Repeat("=", 40))

	expirations, err := client.GetExpirations("SPY")
	if err != nil {
		fmt.Printf("❌ Error: %v\n\n", err)
	} else {
		fmt.Printf("✓ Found %d expirations\n", len(expirations))

		// Find ~45 DTE expiration
		targetDTE := 45
		var selectedExp string
		var selectedDTE int

		fmt.Println("\n  Next 10 expirations (with DTE):")
		displayCount := 0
		for i := 0; i < len(expirations); i++ {
			expDate, err := time.Parse("2006-01-02", expirations[i])
			if err != nil {
				fmt.Printf("Error parsing date %s: %v\n", expirations[i], err)
				continue
			}

			// Load market timezone (ET) for accurate DTE calculation at market close (4 PM ET)
			etLoc, err := time.LoadLocation("America/New_York")
			if err != nil {
				fmt.Printf("Error loading ET timezone: %v\n", err)
				continue
			}

			// Define market-close-aligned times
			expDateET := time.Date(expDate.Year(), expDate.Month(), expDate.Day(), 16, 0, 0, 0, etLoc)
			nowET := time.Now().In(etLoc)
			if nowET.Hour() >= 16 {
				// After close, advance to next trading day for DTE baseline
				nowET = nowET.AddDate(0, 0, 1)
			}
			todayET := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 16, 0, 0, 0, etLoc)
			// Whole-day difference
			dte := int(expDateET.Sub(todayET).Hours() / 24)

			// Skip past or negative DTE expirations
			if dte < 0 {
				continue
			}

			if displayCount < 10 {
				displayCount++
				fmt.Printf("  %d. %s (DTE: %d)\n", displayCount, expirations[i], dte)
			}

			// Select closest to 45 DTE (only consider non-negative futures)
			if selectedExp == "" || absInt(dte-targetDTE) < absInt(selectedDTE-targetDTE) {
				selectedExp = expirations[i]
				selectedDTE = dte
			}
		}
		if selectedExp != "" {
			fmt.Printf("\n  ➜ Selected expiration for ~45 DTE: %s (DTE: %d)\n\n", selectedExp, selectedDTE)
		} else {
			fmt.Printf("\n  ➜ No valid future expirations found. Skipping option chain test.\n\n")
		}

		// Test 3: Get Option Chain
		if selectedExp != "" {
			fmt.Printf("Test 3: Get Option Chain for %s\n", selectedExp)
			fmt.Println("=" + strings.Repeat("=", 40))

			// Create context with timeout for option chain request
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			options, err := client.GetOptionChainCtx(ctx, "SPY", selectedExp, true) // with Greeks
			if err != nil {
				fmt.Printf("❌ Error: %v\n\n", err)
			} else {
				fmt.Printf("✓ Retrieved %d options\n", len(options))

				// Find 16 delta strikes
				putStrike, callStrike, putSymbol, callSymbol := broker.FindStrangleStrikes(options, 0.16)

				if putStrike > 0 && callStrike > 0 {
					fmt.Printf("\n  16Δ Strangle Strikes Found:\n")
					fmt.Printf("  PUT:  $%.2f (%s)\n", putStrike, putSymbol)
					fmt.Printf("  CALL: $%.2f (%s)\n", callStrike, callSymbol)

					// Calculate credit
					credit, err := broker.CalculateStrangleCredit(options, putStrike, callStrike)
					if err != nil {
						fmt.Printf("  Error calculating credit: %v\n", err)
					} else {
						fmt.Printf("  Expected Credit: $%.2f per contract\n", credit)
					}

					// Show details for selected strikes
					fmt.Printf("\n  Option Details:\n")
					for _, opt := range options {
						if eq(opt.Strike, putStrike, broker.StrikeMatchEpsilon) && opt.OptionType == string(broker.OptionTypePut) {
							fmt.Printf("  PUT:  Bid: $%.2f, Ask: $%.2f", opt.Bid, opt.Ask)
							if opt.Greeks != nil {
								fmt.Printf(", Delta: %.3f, IV: %.2f%%", opt.Greeks.Delta, opt.Greeks.MidIV*100)
							}
							fmt.Println()
						}
						if eq(opt.Strike, callStrike, broker.StrikeMatchEpsilon) && opt.OptionType == string(broker.OptionTypeCall) {
							fmt.Printf("  CALL: Bid: $%.2f, Ask: $%.2f", opt.Bid, opt.Ask)
							if opt.Greeks != nil {
								fmt.Printf(", Delta: %.3f, IV: %.2f%%", opt.Greeks.Delta, opt.Greeks.MidIV*100)
							}
							fmt.Println()
						}
					}

					// Test 4: Preview Order (if account ID is set)
					if accountID != "" {
						fmt.Printf("\nTest 4: Preview Strangle Order\n")
						fmt.Println("=" + strings.Repeat("=", 40))

						fmt.Printf("  Order Details:\n")
						fmt.Printf("  - Symbol: SPY\n")
						fmt.Printf("  - Put Strike: $%.2f\n", putStrike)
						fmt.Printf("  - Call Strike: $%.2f\n", callStrike)
						fmt.Printf("  - Quantity: 1 contract\n")
						fmt.Printf("  - Target Credit: $%.2f\n", credit)
						fmt.Printf("  - Type: Credit (Short Strangle)\n")

						// Preview the order - get appropriate tick size
						tickSize := 0.01 // Default fallback

						if ts, err := client.GetTickSize("SPY"); err == nil {
							tickSize = ts
						} else {
							fmt.Printf("  ⚠️  Tick size request failed (using default): %v\n", err)
						}
						px := util.FloorToTick(credit*0.95, tickSize)
						fmt.Printf("  - Limit Price: $%.2f (95%% of mid, rounded to %.4f tick)\n", px, tickSize)

						orderResp, err := client.PlaceStrangleOrder(
							"SPY", putStrike, callStrike, selectedExp,
							1, px, // slightly below mid for better fill (rounded)
							true,      // preview mode
							"day",     // duration
							"preview", // tag
						)

						if err != nil {
							fmt.Printf("\n  ⚠️  Order preview failed: %v\n", err)
						} else {
							fmt.Printf("\n  ✓ Order Preview Successful!\n")
							prettyPrint(orderResp)
						}
					}
				} else {
					fmt.Println("  ⚠️  Could not find suitable 16Δ strikes (Greeks may not be available)")
				}
			}
		}
	}

	// Test 5: Account Balance (if account ID is set)
	if accountID != "" {
		fmt.Println("\nTest 5: Get Account Balance")
		fmt.Println("=" + strings.Repeat("=", 40))

		balance, err := client.GetBalance()
		if err != nil {
			fmt.Printf("❌ Error: %v\n\n", err)
		} else {
			fmt.Printf("✓ Account Balance Retrieved:\n")
			fmt.Printf("  Account Type:        %s\n", balance.Balances.AccountType)
			fmt.Printf("  Total Equity:        $%.2f\n", balance.Balances.TotalEquity)
			
			// Get option buying power using the helper method
			optionBuyingPower, err := balance.GetOptionBuyingPower()
			if err != nil {
				fmt.Printf("  Option Buying Power: Error - %v\n", err)
			} else {
				fmt.Printf("  Option Buying Power: $%.2f\n", optionBuyingPower)
			}
			
			fmt.Printf("  Option Short Value:  $%.2f\n", balance.Balances.OptionShortValue)
			fmt.Printf("  Current Requirement: $%.2f\n", balance.Balances.CurrentRequirement)
			fmt.Printf("  Closed P&L:          $%.2f\n", balance.Balances.ClosePL)
			fmt.Printf("  Total Cash:          $%.2f\n", balance.Balances.TotalCash)
		}

		// Test 6: Get Positions
		fmt.Println("\nTest 6: Get Current Positions")
		fmt.Println("=" + strings.Repeat("=", 40))

		positions, err := client.GetPositions()
		if err != nil {
			fmt.Printf("❌ Error: %v\n\n", err)
		} else {
			if len(positions) == 0 {
				fmt.Println("  No open positions")
			} else {
				fmt.Printf("✓ Found %d positions:\n", len(positions))
				for i, pos := range positions {
					posType := "LONG"
					if pos.Quantity < 0 {
						posType = "SHORT"
					}
					unit := "shares"
					if isOptionSymbol(pos.Symbol) {
						unit = "contracts"
					}
					q := pos.Quantity
					if math.Abs(q-math.Round(q)) < 1e-6 {
						fmt.Printf("  %d. %s: %.0f %s (%s), Cost: $%.2f\n", i+1, pos.Symbol, q, unit, posType, pos.CostBasis)
					} else {
						fmt.Printf("  %d. %s: %.2f %s (%s), Cost: $%.2f\n", i+1, pos.Symbol, q, unit, posType, pos.CostBasis)
					}
				}

				// Check for strangle
				hasStrangle, putPos, callPos := broker.CheckStranglePosition(positions, "SPY")
				if hasStrangle {
					fmt.Printf("\n  ✓ SPY Strangle Detected!\n")
					fmt.Printf("    PUT:  %s (%.0f contracts)\n", putPos.Symbol, putPos.Quantity)
					fmt.Printf("    CALL: %s (%.0f contracts)\n", callPos.Symbol, callPos.Quantity)
				}
			}
		}
	}

	fmt.Println("\n=== Test Suite Complete ===")
	fmt.Println("\nNext Steps:")
	if accountID == "" {
		fmt.Println("1. Set TRADIER_ACCOUNT_ID to enable account tests")
		fmt.Println("2. Review the option chain data above")
	} else {
		fmt.Println("1. Review the test results")
		fmt.Println("2. Verify option symbols and Greeks are correct")
		fmt.Println("3. Test with a real order (remove preview flag)")
	}
}

func formatNumber(n int64) string {
	if n >= 1000000 {
		// Round to nearest 0.1M
		millions := float64(n) / 1000000
		rounded := math.Round(millions*10) / 10
		return fmt.Sprintf("%.1fM", rounded)
	} else if n >= 1000 {
		// Round to nearest 0.1K
		thousands := float64(n) / 1000
		rounded := math.Round(thousands*10) / 10
		return fmt.Sprintf("%.1fK", rounded)
	}
	return fmt.Sprintf("%d", n)
}

func prettyPrint(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Printf("%s\n", string(b))
}

func maskAPIKey(apiKey string) string {
	const minLength = 12 // Minimum length to show partial key
	const showFirst = 4  // Show first 4 characters
	const showLast = 4   // Show last 4 characters

	if apiKey == "" {
		return "<redacted>"
	}

	if len(apiKey) < minLength {
		return "<redacted>"
	}

	return fmt.Sprintf("%s...%s", apiKey[:showFirst], apiKey[len(apiKey)-showLast:])
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func eq(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// isOptionSymbol performs a robust OPRA-style check: TICKER + YYMMDD + [C|P] + strike
// Example: SPY240920P00450000
// Uses regex pattern: ^[A-Z]{1,6}\d{6}[CP]\d{8}$
func isOptionSymbol(s string) bool {
	return optionSymbolRegex.MatchString(strings.ToUpper(strings.TrimSpace(s)))
}

// readConfigCredentials reads API key and account ID from config.yaml
func readConfigCredentials() (apiKey, accountID string, err error) {
	// Try to read config.yaml from current directory or parent directory
	configPaths := []string{"config.yaml", "../config.yaml", "../../config.yaml"}

	var configFile string
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			configFile = path
			break
		}
	}

	if configFile == "" {
		return "", "", fmt.Errorf("config.yaml not found in current or parent directories")
	}

	data, err := os.ReadFile(configFile) // #nosec G304
	if err != nil {
		return "", "", fmt.Errorf("failed to read config file: %v", err)
	}

	// Simple YAML parsing to extract broker credentials
	configContent := string(data)
	lines := strings.Split(configContent, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "api_key:") && !strings.HasPrefix(line, "#") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				value := strings.TrimSpace(strings.Join(parts[1:], ":"))
				// Remove comment if present
				if idx := strings.Index(value, "#"); idx != -1 {
					value = strings.TrimSpace(value[:idx])
				}
				apiKey = strings.Trim(value, `"`) // Remove quotes if present
			}
		}
		if strings.Contains(line, "account_id:") && !strings.HasPrefix(line, "#") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				value := strings.TrimSpace(strings.Join(parts[1:], ":"))
				// Remove comment if present
				if idx := strings.Index(value, "#"); idx != -1 {
					value = strings.TrimSpace(value[:idx])
				}
				accountID = strings.Trim(value, `"`) // Remove quotes if present
			}
		}

		// Check if we're past the broker section
		if strings.Contains(line, "strategy:") {
			break
		}
	}

	if apiKey == "" || apiKey == "YOUR_SANDBOX_API_KEY_HERE" {
		return "", "", fmt.Errorf("API key not found or is placeholder in config.yaml")
	}

	if accountID == "" || accountID == "YOUR_ACCOUNT_ID_HERE" {
		return "", "", fmt.Errorf("account ID not found or is placeholder in config.yaml")
	}

	return apiKey, accountID, nil
}
