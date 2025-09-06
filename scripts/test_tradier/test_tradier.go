package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
)

func main() {
	var sandbox bool
	flag.BoolVar(&sandbox, "sandbox", true, "Use Tradier sandbox endpoints (default: true)")
	flag.Parse()

	fmt.Println("=== Tradier API Complete Test Suite ===")
	fmt.Println()

	// Check for API credentials
	apiKey := os.Getenv("TRADIER_API_KEY")
	accountID := os.Getenv("TRADIER_ACCOUNT_ID")

	if apiKey == "" {
		fmt.Println("❌ TRADIER_API_KEY not set")
		fmt.Println("\nSetup Instructions:")
		fmt.Println("1. Go to https://developer.tradier.com/")
		fmt.Println("2. Sign up for a free account")
		fmt.Println("3. Get your sandbox API token")
		fmt.Println("4. Get your sandbox account ID")
		fmt.Println("5. Export them:")
		fmt.Println("   export TRADIER_API_KEY='your_token_here'")
		fmt.Println("   export TRADIER_ACCOUNT_ID='your_account_id'")
		os.Exit(1)
	}

	if accountID == "" {
		fmt.Println("⚠️  TRADIER_ACCOUNT_ID not set, some tests will be skipped")
	}

	// Initialize client
	client := broker.NewTradierClient(apiKey, accountID, sandbox, true /* useOTOCO */, 0.5 /* profitTarget */)
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
		for i := 0; i < 10 && i < len(expirations); i++ {
			expDate, err := time.Parse("2006-01-02", expirations[i])
			if err != nil {
				fmt.Printf("Error parsing date %s: %v\n", expirations[i], err)
				continue
			}
			dte := int(time.Until(expDate).Hours() / 24)
			fmt.Printf("  %d. %s (DTE: %d)\n", i+1, expirations[i], dte)

			// Select closest to 45 DTE
			if selectedExp == "" || abs(dte-targetDTE) < abs(selectedDTE-targetDTE) {
				selectedExp = expirations[i]
				selectedDTE = dte
			}
		}
		fmt.Printf("\n  ➜ Selected expiration for ~45 DTE: %s (DTE: %d)\n\n", selectedExp, selectedDTE)

		// Test 3: Get Option Chain
		if selectedExp != "" {
			fmt.Printf("Test 3: Get Option Chain for %s\n", selectedExp)
			fmt.Println("=" + strings.Repeat("=", 40))

			options, err := client.GetOptionChain("SPY", selectedExp, true) // with Greeks
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
					credit := broker.CalculateStrangleCredit(options, putStrike, callStrike)
					fmt.Printf("  Expected Credit: $%.2f per contract\n", credit)

					// Show details for selected strikes
					fmt.Printf("\n  Option Details:\n")
					for _, opt := range options {
						if opt.Strike == putStrike && opt.OptionType == "put" {
							fmt.Printf("  PUT:  Bid: $%.2f, Ask: $%.2f", opt.Bid, opt.Ask)
							if opt.Greeks != nil {
								fmt.Printf(", Delta: %.3f, IV: %.2f%%", opt.Greeks.Delta, opt.Greeks.MidIV*100)
							}
							fmt.Println()
						}
						if opt.Strike == callStrike && opt.OptionType == "call" {
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

						// Preview the order
						orderResp, err := client.PlaceStrangleOrder(
							"SPY", putStrike, callStrike, selectedExp,
							1, credit*0.95, // slightly below mid for better fill
							true, // preview mode
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
			fmt.Printf("  Total Equity:        $%.2f\n", balance.Balances.TotalEquity)
			fmt.Printf("  Option Buying Power: $%.2f\n", balance.Balances.OptionBuyingPower)
			fmt.Printf("  Option Short Value:  $%.2f\n", balance.Balances.OptionShortValue)
			fmt.Printf("  Current Requirement: $%.2f\n", balance.Balances.CurrentRequirement)
			fmt.Printf("  Closed P&L:          $%.2f\n", balance.Balances.ClosedPL)
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
					fmt.Printf("  %d. %s: %.0f shares (%s), Cost: $%.2f\n",
						i+1, pos.Symbol, abs(pos.Quantity), posType, pos.CostBasis)
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
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	} else if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func prettyPrint(v interface{}) {
	b, err := json.MarshalIndent(v, "  ", "  ")
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

func abs[T float64 | int](x T) T {
	if x < 0 {
		return -x
	}
	return x
}
