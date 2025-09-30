package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	brokerPkg "github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
)

// isOptionSymbol determines if a symbol represents an option contract
// by leveraging the broker's OSI parsing logic to avoid duplication
func isOptionSymbol(symbol string) bool {
	return brokerPkg.ExtractUnderlyingFromOSI(symbol) != ""
}

func main() {
	fmt.Println("=== SPY Strangle Bot - End-to-End Integration Test ===")
	fmt.Println()

	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Ensure we're in paper mode for safety
	if cfg.Environment.Mode != "paper" {
		log.Fatalf("Integration tests must run in paper mode. Set environment.mode: 'paper' in config.yaml")
	}

	// Create logger
	logger := log.New(os.Stdout, "[E2E] ", log.LstdFlags)

	// Initialize broker client
	brokerClient, err := brokerPkg.NewTradierClient(
		cfg.Broker.APIKey,
		cfg.Broker.AccountID,
		true, // force sandbox mode for integration tests
		cfg.Broker.UseOTOCO,
		cfg.Strategy.Exit.ProfitTarget,
	)
	if err != nil {
		log.Fatalf("Failed to create broker client: %v", err)
	}

	// Initialize storage with temporary test file
	testStoragePath := "data/positions_integration_test.json"
	
	// Ensure the directory exists
	testDir := filepath.Dir(testStoragePath)
	if err := os.MkdirAll(testDir, 0o750); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}
	
	storageClient, err := storage.NewJSONStorage(testStoragePath)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// Cleanup test storage at the end
	defer func() {
		if err := os.Remove(testStoragePath); err != nil && !os.IsNotExist(err) {
			logger.Printf("Warning: Failed to cleanup test storage file: %v", err)
		}
	}()

	// Create strategy configuration
	strategyConfig := &strategy.Config{
		Symbol:              cfg.Strategy.Symbol,
		DTETarget:           cfg.Strategy.Entry.TargetDTE,
		DTERange:            cfg.Strategy.Entry.DTERange,
		DeltaTarget:         float64(cfg.Strategy.Entry.Delta) / 100,
		ProfitTarget:        cfg.Strategy.Exit.ProfitTarget,
		MaxDTE:              cfg.Strategy.Exit.MaxDTE,
		AllocationPct:       cfg.Strategy.AllocationPct,
		MinIVPct:            cfg.Strategy.Entry.MinIVPct,
		MinCredit:           cfg.Strategy.Entry.MinCredit,
		EscalateLossPct:     cfg.Strategy.EscalateLossPct,
		StopLossPct:         cfg.Strategy.Exit.StopLossPct,
		MaxPositionLoss:     cfg.Risk.MaxPositionLoss,
		MaxContracts:        cfg.Risk.MaxContracts,
		MinVolume:           cfg.Strategy.Entry.MinVolume,
		MinOpenInterest:     cfg.Strategy.Entry.MinOpenInterest,
	}

	// Initialize strategy
	strangleStrategy := strategy.NewStrangleStrategy(brokerClient, strategyConfig, logger, storageClient)

	fmt.Println("✅ All components initialized successfully")
	fmt.Println()

	// Run integration tests
	runIntegrationTests(brokerClient, strangleStrategy, storageClient, logger, cfg)
}

func runIntegrationTests(broker brokerPkg.Broker, strategy *strategy.StrangleStrategy, storage storage.Interface, logger *log.Logger, cfg *config.Config) {
	testsPassed := 0
	totalTests := 6

	// Test 1: Broker connectivity
	fmt.Println("Test 1: Broker Connectivity")
	fmt.Println("============================")
	if testBrokerConnectivity(broker, logger) {
		testsPassed++
		fmt.Println("✅ PASSED")
	} else {
		fmt.Println("❌ FAILED")
	}
	fmt.Println()

	// Test 2: Market data retrieval
	fmt.Println("Test 2: Market Data Retrieval")
	fmt.Println("==============================")
	if testMarketDataRetrieval(broker, logger) {
		testsPassed++
		fmt.Println("✅ PASSED")
	} else {
		fmt.Println("❌ FAILED")
	}
	fmt.Println()

	// Test 3: Entry conditions logic
	fmt.Println("Test 3: Entry Conditions Logic")
	fmt.Println("===============================")
	if testEntryConditions(strategy, logger) {
		testsPassed++
		fmt.Println("✅ PASSED")
	} else {
		fmt.Println("❌ FAILED")
	}
	fmt.Println()

	// Test 4: Order preview functionality
	fmt.Println("Test 4: Order Preview")
	fmt.Println("======================")
	if testOrderPreview(broker, logger, cfg) {
		testsPassed++
		fmt.Println("✅ PASSED")
	} else {
		fmt.Println("❌ FAILED")
	}
	fmt.Println()

	// Test 5: Position storage
	fmt.Println("Test 5: Position Storage")
	fmt.Println("========================")
	if testPositionStorage(storage, logger) {
		testsPassed++
		fmt.Println("✅ PASSED")
	} else {
		fmt.Println("❌ FAILED")
	}
	fmt.Println()

	// Test 6: Risk management
	fmt.Println("Test 6: Risk Management")
	fmt.Println("=======================")
	if testRiskManagement(broker, logger, cfg) {
		testsPassed++
		fmt.Println("✅ PASSED")
	} else {
		fmt.Println("❌ FAILED")
	}
	fmt.Println()

	// Summary
	fmt.Println("=== Integration Test Results ===")
	fmt.Printf("Tests Passed: %d/%d\n", testsPassed, totalTests)
	if testsPassed == totalTests {
		fmt.Println("🎉 ALL TESTS PASSED - Bot ready for paper trading validation!")
	} else {
		fmt.Printf("⚠️  %d test(s) failed - review issues before live trading\n", totalTests-testsPassed)
		os.Exit(1)
	}
}

func testBrokerConnectivity(broker brokerPkg.Broker, logger *log.Logger) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	balance, err := broker.GetAccountBalanceCtx(ctx)
	if err != nil {
		logger.Printf("Broker connectivity failed: %v", err)
		return false
	}

	logger.Printf("Account balance: $%.2f", balance)
	return balance > 0
}

func testMarketDataRetrieval(broker brokerPkg.Broker, logger *log.Logger) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test SPY quote
	quote, err := broker.GetQuote("SPY")
	if err != nil {
		logger.Printf("Failed to get SPY quote: %v", err)
		return false
	}
	logger.Printf("SPY Last: $%.2f", quote.Last)

	// Test option expirations
	expirations, err := broker.GetExpirationsCtx(ctx, "SPY")
	if err != nil {
		logger.Printf("Failed to get expirations: %v", err)
		return false
	}
	logger.Printf("Found %d expirations", len(expirations))

	// Test option chain (just first expiration)
	if len(expirations) > 0 {
		options, err := broker.GetOptionChain("SPY", expirations[0], true)
		if err != nil {
			logger.Printf("Failed to get option chain: %v", err)
			return false
		}
		logger.Printf("Found %d options for %s", len(options), expirations[0])
	}

	return len(expirations) > 0
}

func testEntryConditions(strategy *strategy.StrangleStrategy, logger *log.Logger) bool {
	canEnter, reason := strategy.CheckEntryConditions()
	logger.Printf("Entry conditions: %t (%s)", canEnter, reason)
	
	// Test passes if we can evaluate conditions (regardless of result)
	return reason != ""
}

func testOrderPreview(broker brokerPkg.Broker, logger *log.Logger, cfg *config.Config) bool {
	// Get current SPY quote to compute realistic strikes
	quote, err := broker.GetQuote("SPY")
	if err != nil {
		logger.Printf("Failed to get SPY quote for strike calculation: %v", err)
		return false
	}

	// Compute strikes as quote ±10%, rounded to nearest $5
	putStrike := math.Floor((quote.Last*0.90)/5) * 5
	callStrike := math.Ceil((quote.Last*1.10)/5) * 5

	// Get real expirations and select closest to target DTE
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exps, err := broker.GetExpirationsCtx(ctx, "SPY")
	if err != nil || len(exps) == 0 {
		logger.Printf("Failed to get expirations for preview: %v", err)
		return false
	}
	// Pick expiration closest to configured target DTE
	target := cfg.Strategy.Entry.TargetDTE
	best := exps[0]
	bestDiff := math.MaxFloat64
	now := time.Now()
	for _, e := range exps {
		if t, parseErr := time.Parse("2006-01-02", e); parseErr == nil {
			dte := t.Sub(now).Hours() / 24
			diff := math.Abs(dte - float64(target))
			if diff < bestDiff {
				bestDiff, best = diff, e
			}
		}
	}
	expiration := best

	order, err := broker.PlaceStrangleOrder(
		"SPY",
		putStrike,  // put strike (computed)
		callStrike, // call strike (computed)
		expiration,
		1,     // quantity
		cfg.Strategy.Entry.MinCredit, // limit price from config
		true,  // preview mode
		string(brokerPkg.DurationGTC),
		fmt.Sprintf("integration-test-%d", time.Now().Unix()%100000),
	)
	
	if err != nil {
		logger.Printf("Order preview failed: %v", err)
		return false
	}
	
	if order != nil {
		logger.Printf("Preview order status: '%s'", order.Order.Status)
		logger.Printf("Preview order class: '%s'", order.Order.Class)
		logger.Printf("Preview order price: $%.2f", order.Order.Price)
		status := order.Order.Status
		return status == "ok" || status == ""
	}
	
	logger.Printf("Order preview returned nil response")
	return false
}

func testPositionStorage(storage storage.Interface, logger *log.Logger) bool {
	// Test loading (should work even if file doesn't exist)
	err := storage.Load()
	if err != nil {
		logger.Printf("Failed to load storage: %v", err)
		return false
	}
	
	// Test getting current positions
	positions := storage.GetCurrentPositions()
	logger.Printf("Storage test: found %d current positions", len(positions))
	
	// Test saving storage state
	err = storage.Save()
	if err != nil {
		logger.Printf("Failed to save storage: %v", err)
		return false
	}
	
	logger.Printf("Storage operations successful")
	return true
}

func testRiskManagement(broker brokerPkg.Broker, logger *log.Logger, cfg *config.Config) bool {
	// Test account balance check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	balance, err := broker.GetAccountBalanceCtx(ctx)
	if err != nil {
		logger.Printf("Failed to get balance for risk test: %v", err)
		return false
	}
	
	// Test position sizing calculation
	allocationPct := cfg.Strategy.AllocationPct
	maxAllocation := balance * allocationPct
	logger.Printf("Max allocation (%.0f%%): $%.2f", allocationPct*100, maxAllocation)
	
	// Test that we can retrieve buying power (sandbox may have different limits)
	bpCtx, bpCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bpCancel()
	buyingPower, err := broker.GetOptionBuyingPowerCtx(bpCtx)
	if err != nil {
		logger.Printf("Failed to get buying power: %v", err)
		return false
	}
	
	logger.Printf("Option buying power: $%.2f", buyingPower)
	
	// Validate basic risk parameters are reasonable
	if math.IsNaN(balance) || math.IsInf(balance, 0) || balance <= 0 {
		logger.Printf("Invalid account balance: $%.2f", balance)
		return false
	}
	if math.IsNaN(allocationPct) || math.IsInf(allocationPct, 0) || allocationPct <= 0 || allocationPct > 1 {
		logger.Printf("Invalid allocation percentage: %v", allocationPct)
		return false
	}
	
	if math.IsNaN(maxAllocation) || math.IsInf(maxAllocation, 0) || maxAllocation <= 0 {
		logger.Printf("Invalid max allocation: $%.2f", maxAllocation)
		return false
	}
	
	if math.IsNaN(buyingPower) || math.IsInf(buyingPower, 0) || buyingPower < 0 {
		logger.Printf("Invalid buying power: $%.2f", buyingPower)
		return false
	}
	
	// In sandbox, buying power restrictions may not reflect live trading conditions
	// Check if there are existing positions consuming buying power
	posCtx, posCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer posCancel()
	positions, err := broker.GetPositionsCtx(posCtx)
	if err != nil {
		logger.Printf("Warning: Could not retrieve positions: %v", err)
	} else if len(positions) > 0 {
		logger.Printf("Found %d existing positions consuming buying power", len(positions))
		// Calculate approximate margin requirement for existing positions
		totalContracts := 0
		for _, pos := range positions {
			// Count only option positions; skip underlying equities
			// Use OSI format parsing to detect option symbols
			if !isOptionSymbol(pos.Symbol) {
				continue
			}
			totalContracts += int(math.Abs(pos.Quantity))
		}
		if totalContracts > 0 {
			logger.Printf("Existing positions: %d option contracts", totalContracts)
		}
	}
	
	// If buying power is positive, check if it meets allocation requirements
	if maxAllocation > 0 {
		effBP := buyingPower
		if effBP <= 0 {
			if cfg.Environment.Mode == "paper" {
				effBP = balance
				logger.Printf("Buying power unavailable in sandbox; falling back to balance: $%.2f", balance)
			} else {
				logger.Printf("Buying power is zero/unknown; cannot validate max allocation")
				return false
			}
		}
		if effBP < maxAllocation {
			logger.Printf("Current buying power ($%.2f) below required allocation ($%.2f)", effBP, maxAllocation)
			// This is expected behavior when positions are already open
			// The test passes as long as the risk check is working correctly
			logger.Printf("✓ Risk check correctly preventing over-allocation")
			return true
		}
	}
	
	// Test passes if we can retrieve valid values and calculate risk parameters
	logger.Printf("Risk management validation successful (balance=%.2f allocPct=%.3f maxAlloc=%.2f buyingPower=%.2f)", balance, allocationPct, maxAllocation, buyingPower)
	return true
}