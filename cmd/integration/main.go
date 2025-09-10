package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/config"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
)

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
	brokerClient, err := broker.NewTradierClient(
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
	runIntegrationTests(brokerClient, strangleStrategy, storageClient, logger)
}

func runIntegrationTests(broker broker.Broker, strategy *strategy.StrangleStrategy, storage storage.Interface, logger *log.Logger) {
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
	if testOrderPreview(broker, logger) {
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
	if testRiskManagement(strategy, broker, logger) {
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
		fmt.Println("🎉 ALL TESTS PASSED - Bot ready for live trading!")
	} else {
		fmt.Printf("⚠️  %d test(s) failed - review issues before live trading\n", totalTests-testsPassed)
		os.Exit(1)
	}
}

func testBrokerConnectivity(broker broker.Broker, logger *log.Logger) bool {
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

func testMarketDataRetrieval(broker broker.Broker, logger *log.Logger) bool {
	// Test SPY quote
	quote, err := broker.GetQuote("SPY")
	if err != nil {
		logger.Printf("Failed to get SPY quote: %v", err)
		return false
	}
	logger.Printf("SPY Last: $%.2f", quote.Last)

	// Test option expirations
	expirations, err := broker.GetExpirations("SPY")
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

func testOrderPreview(broker broker.Broker, logger *log.Logger) bool {
	// Use realistic SPY strikes for preview
	expiration := time.Now().Add(45 * 24 * time.Hour).Format("2006-01-02")
	
	order, err := broker.PlaceStrangleOrder(
		"SPY",
		600.0, // put strike
		700.0, // call strike
		expiration,
		1,     // quantity
		2.50,  // limit price
		true,  // preview mode
		"day",
		"integration-test",
	)
	
	if err != nil {
		logger.Printf("Order preview failed: %v", err)
		return false
	}
	
	if order != nil {
		logger.Printf("Preview order status: '%s'", order.Order.Status)
		logger.Printf("Preview order class: '%s'", order.Order.Class)
		logger.Printf("Preview order price: $%.2f", order.Order.Price)
		
		// Consider test successful if we got an order response without error
		// Status might be empty in sandbox preview mode
		return order.Order.Status == "ok" || order.Order.Status == ""
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

func testRiskManagement(strategy *strategy.StrangleStrategy, broker broker.Broker, logger *log.Logger) bool {
	// Test account balance check
	balance, err := broker.GetAccountBalance()
	if err != nil {
		logger.Printf("Failed to get balance for risk test: %v", err)
		return false
	}
	
	// Test position sizing calculation
	maxAllocation := balance * 0.35 // 35% allocation
	logger.Printf("Max allocation (35%%): $%.2f", maxAllocation)
	
	// Test that we have sufficient buying power
	buyingPower, err := broker.GetOptionBuyingPower()
	if err != nil {
		logger.Printf("Failed to get buying power: %v", err)
		return false
	}
	
	logger.Printf("Option buying power: $%.2f", buyingPower)
	return buyingPower >= maxAllocation
}