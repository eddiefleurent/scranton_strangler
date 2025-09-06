package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/eddie/spy-strangle-bot/internal/broker"
)

func main() {
	var preview bool
	flag.BoolVar(&preview, "preview", true, "Preview order without placing (default: true)")
	flag.Parse()

	// Get API key from environment
	apiKey := os.Getenv("TRADIER_API_KEY")
	if apiKey == "" {
		log.Fatal("TRADIER_API_KEY environment variable not set")
	}

	// Get account ID (you'll need to set this)
	accountID := os.Getenv("TRADIER_ACCOUNT_ID")
	if accountID == "" {
		// Use a default sandbox account ID if not set
		accountID = "VA00000000" // Replace with your sandbox account
		fmt.Printf("Using default sandbox account ID: %s\n", accountID)
	}

	fmt.Println("=== Testing Tradier OTOCO Order ===")
	fmt.Printf("Account ID: %s\n", accountID)
	fmt.Printf("Preview Mode: %v\n", preview)
	fmt.Println()

	// Create API client
	client := broker.NewTradierAPI(apiKey, accountID, true) // true = sandbox

	// 1. Get SPY quote
	fmt.Println("1. Getting SPY quote...")
	quote, err := client.GetQuote("SPY")
	if err != nil {
		log.Fatalf("Failed to get quote: %v", err)
	}
	fmt.Printf("   SPY Last: $%.2f\n", quote.Last)

	// 2. Get expirations
	fmt.Println("\n2. Finding 45 DTE expiration...")
	expirations, err := client.GetExpirations("SPY")
	if err != nil {
		log.Fatalf("Failed to get expirations: %v", err)
	}

	// Find expiration closest to 45 DTE
	targetDate := time.Now().AddDate(0, 0, 45)
	var bestExpiration string
	bestDiff := 999

	for _, exp := range expirations {
		expDate, _ := time.Parse("2006-01-02", exp)
		diff := int(expDate.Sub(targetDate).Hours() / 24)
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			bestExpiration = exp
		}
	}

	expDate, _ := time.Parse("2006-01-02", bestExpiration)
	dte := int(expDate.Sub(time.Now()).Hours() / 24)
	fmt.Printf("   Selected expiration: %s (%d DTE)\n", bestExpiration, dte)

	// 3. Get option chain with Greeks
	fmt.Println("\n3. Getting option chain with Greeks...")
	options, err := client.GetOptionChain("SPY", bestExpiration, true)
	if err != nil {
		log.Fatalf("Failed to get option chain: %v", err)
	}
	fmt.Printf("   Found %d options\n", len(options))

	// 4. Find 16 delta strikes
	fmt.Println("\n4. Finding 16 delta strikes...")
	putStrike, callStrike, putSymbol, callSymbol := broker.FindStrangleStrikes(options, 0.16)

	if putStrike == 0 || callStrike == 0 {
		log.Fatal("Could not find suitable strikes")
	}

	fmt.Printf("   Put Strike: $%.0f (%s)\n", putStrike, putSymbol)
	fmt.Printf("   Call Strike: $%.0f (%s)\n", callStrike, callSymbol)

	// 5. Calculate expected credit
	credit := broker.CalculateStrangleCredit(options, putStrike, callStrike)
	fmt.Printf("   Expected Credit: $%.2f per contract\n", credit)

	// 6. Test OTOCO order
	fmt.Println("\n5. Testing OTOCO strangle order...")
	fmt.Println("   Order Type: OTOCO (One-Triggers-One-Cancels-Other)")
	fmt.Println("   Primary Order: Sell strangle for credit")
	fmt.Printf("   Exit Order: Buy back at 50%% profit ($%.2f)\n", credit*0.5)

	if preview {
		fmt.Println("\n   ⚠️  PREVIEW MODE - Order will not be placed")
	} else {
		fmt.Println("\n   ⚠️  LIVE MODE - Order WILL be placed!")
		fmt.Println("   Waiting 5 seconds... Press Ctrl+C to cancel")
		time.Sleep(5 * time.Second)
	}

	// Place OTOCO order
	orderResp, err := client.PlaceStrangleOTOCO(
		"SPY",
		putStrike,
		callStrike,
		bestExpiration,
		1, // 1 contract
		credit,
		0.5, // 50% profit target
		preview,
	)

	if err != nil {
		log.Fatalf("Failed to place OTOCO order: %v", err)
	}

	fmt.Println("\n6. Order Response:")
	respJSON, _ := json.MarshalIndent(orderResp, "   ", "  ")
	fmt.Println(string(respJSON))

	if preview {
		fmt.Println("\n✅ OTOCO order preview successful!")
		fmt.Println("   The order would:")
		fmt.Println("   1. Open a strangle at the specified strikes")
		fmt.Println("   2. Automatically place a GTC exit order at 50% profit")
		fmt.Println("   3. Exit order remains active until filled or cancelled")
	} else {
		fmt.Printf("\n✅ OTOCO order placed! Order ID: %d\n", orderResp.Order.ID)
		fmt.Println("   Exit order is now active and will close at 50% profit")
	}
}
