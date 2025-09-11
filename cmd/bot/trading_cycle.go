package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
	"github.com/eddiefleurent/scranton_strangler/internal/util"
	"github.com/google/uuid"
)

// TradingCycle encapsulates the main trading logic
type TradingCycle struct {
	bot        *Bot
	reconciler *Reconciler
}

// NewTradingCycle creates a new trading cycle handler
func NewTradingCycle(bot *Bot) *TradingCycle {
	return &TradingCycle{
		bot:        bot,
		reconciler: NewReconciler(bot.broker, bot.storage, bot.logger),
	}
}

// Run executes one trading cycle
func (tc *TradingCycle) Run() {
	now := time.Now()
	if tc.bot.nyLocation != nil {
		now = now.In(tc.bot.nyLocation)
	} else {
		tc.bot.logger.Printf("Warning: NY timezone not cached, using system time")
	}

	// Check market schedule
	if !tc.checkMarketSchedule(now) {
		return
	}

	// Check real-time market status
	isMarketOpen, marketState := tc.checkMarketStatus()
	if !tc.shouldRunCycle(isMarketOpen, marketState) {
		return
	}

	tc.bot.logger.Println("Starting trading cycle...")

	// Get and reconcile positions
	positions := tc.bot.storage.GetCurrentPositions()
	tc.bot.logger.Printf("Currently managing %d position(s)", len(positions))
	positions = tc.reconciler.ReconcilePositions(positions)

	// Check exits for existing positions
	tc.checkExitConditions(positions)

	// Check for adjustments if enabled
	if tc.bot.config.Strategy.Adjustments.Enabled && isMarketOpen {
		tc.checkAdjustments(positions)
	}

	// Check for new entries
	if isMarketOpen {
		tc.checkEntryConditions(positions)
	}

	tc.bot.logger.Println("Trading cycle complete")
}

func (tc *TradingCycle) checkMarketSchedule(now time.Time) bool {
	todaySchedule, err := tc.bot.getTodaysMarketSchedule()
	if err != nil {
		tc.bot.logger.Printf("Warning: Could not get today's market schedule: %v", err)
		return true // Continue with real-time check
	}

	if todaySchedule.Status == "closed" {
		tc.bot.logger.Printf("Market is officially CLOSED today: %s", todaySchedule.Description)
		tc.bot.logger.Println("Trading cycle skipped - market holiday")
		return false
	}

	if todaySchedule.Open != nil {
		tc.bot.logger.Printf("Official market hours today: %s - %s",
			todaySchedule.Open.Start, todaySchedule.Open.End)
	}
	return true
}

func (tc *TradingCycle) checkMarketStatus() (bool, string) {
	marketClock, err := tc.bot.broker.GetMarketClock(false)

	if err == nil && marketClock != nil {
		state := marketClock.Clock.State
		isOpen := state == "open"
		tc.bot.logger.Printf("Real-time market status: %s", state)
		return isOpen, state
	}

	// Fallback to config-based hours
	if err != nil {
		tc.bot.logger.Printf("Warning: Could not get market clock: %v, falling back to config-based hours", err)
	}
	
	now := time.Now()
	if tc.bot.nyLocation != nil {
		now = now.In(tc.bot.nyLocation)
	}
	
	isOpen, err := tc.bot.config.IsWithinTradingHours(now)
	if err != nil {
		tc.bot.logger.Printf("Warning: Could not determine trading hours: %v, assuming market closed", err)
		return false, "unknown"
	}
	
	tc.bot.logger.Printf("Using config-based market hours: open=%t", isOpen)
	return isOpen, "unknown"
}

func (tc *TradingCycle) shouldRunCycle(isMarketOpen bool, marketState string) bool {
	if !isMarketOpen {
		if !tc.bot.config.Schedule.AfterHoursCheck {
			tc.bot.logger.Printf("Market is %s, skipping cycle", marketState)
			return false
		}
		tc.bot.logger.Printf("Market is %s, running after-hours check for existing positions only", marketState)
	}
	return true
}

func (tc *TradingCycle) checkExitConditions(positions []models.Position) {
	for _, position := range positions {
		now := time.Now()
		if tc.bot.nyLocation != nil {
			now = now.In(tc.bot.nyLocation)
		}
		dte := int(position.Expiration.Sub(now).Hours() / 24)
		tc.bot.logger.Printf("Checking position %s (%.2f/%.2f, %d DTE)",
			shortID(position.ID), position.PutStrike, position.CallStrike, dte)

		posCopy := position
		shouldExit, reason := tc.bot.strategy.CheckExitConditions(&posCopy)
		if shouldExit {
			tc.bot.logger.Printf("Exit signal for position %s: %s", shortID(position.ID), reason)
			tc.executeExit(&posCopy, reason)
		} else {
			tc.bot.logger.Printf("No exit conditions met for position %s", shortID(position.ID))
		}
	}
}

func (tc *TradingCycle) checkAdjustments(positions []models.Position) {
	for _, position := range positions {
		posCopy := position
		tc.checkAdjustmentsForPosition(&posCopy)
	}
}

func (tc *TradingCycle) checkEntryConditions(positions []models.Position) {
	maxPositions := tc.bot.config.Risk.MaxPositions
	if maxPositions <= 0 {
		maxPositions = 1
	}

	if len(positions) >= maxPositions {
		tc.bot.logger.Printf("Maximum positions (%d) reached, not checking for new entries", maxPositions)
		return
	}

	tc.bot.logger.Printf("Have %d/%d positions, checking entry conditions...", len(positions), maxPositions)

	// Additional reconcile before opening new trades
	tc.bot.logger.Printf("Running additional reconcile check before opening new trades...")
	positions = tc.reconciler.ReconcilePositions(positions)

	if len(positions) >= maxPositions {
		tc.bot.logger.Printf("Position limit reached after reconcile (%d/%d), skipping new entries", 
			len(positions), maxPositions)
		return
	}

	// Check buying power
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	buyingPower, err := tc.bot.broker.GetOptionBuyingPowerCtx(ctx)
	if err != nil {
		tc.bot.logger.Printf("Warning: Could not get option buying power: %v", err)
		buyingPower = 0
	}

	tc.bot.logger.Printf("Available option buying power: $%.2f", buyingPower)

	if buyingPower <= 1000 {
		tc.bot.logger.Printf("Insufficient buying power for new positions")
		return
	}

	// Check entry conditions
	canEnter, reason := tc.bot.strategy.CheckEntryConditions()
	if !canEnter {
		tc.bot.logger.Printf("Entry conditions not met: %s", reason)
		return
	}

	tc.bot.logger.Printf("Entry signal: %s", reason)

	// Open new positions
	remainingSlots := maxPositions - len(positions)
	maxNewPositions := tc.bot.config.Strategy.MaxNewPositionsPerCycle
	if maxNewPositions <= 0 {
		maxNewPositions = 1
	}
	for i := 0; i < remainingSlots && i < maxNewPositions; i++ {
		tc.executeEntry()
	}
}

func (tc *TradingCycle) executeEntry() {
	tc.bot.logger.Println("Executing entry...")

	// Find strikes
	order, err := tc.bot.strategy.FindStrangleStrikes()
	if err != nil {
		tc.bot.logger.Printf("Failed to find strikes: %v", err)
		return
	}

	tc.bot.logger.Printf("Found strangle: Put %.0f / Call %.0f, Credit: $%.2f",
		order.PutStrike, order.CallStrike, order.Credit)

	// Risk check
	if order.Quantity > tc.bot.config.Risk.MaxContracts {
		order.Quantity = tc.bot.config.Risk.MaxContracts
		tc.bot.logger.Printf("Position size limited to %d contracts", order.Quantity)
	}

	if order.Quantity <= 0 {
		tc.bot.logger.Printf("ERROR: Computed order size is non-positive (%d), aborting", order.Quantity)
		return
	}

	// Parse expiration
	expirationTime, err := time.Parse("2006-01-02", order.Expiration)
	if err != nil {
		tc.bot.logger.Printf("Failed to parse expiration date %q: %v", order.Expiration, err)
		return
	}

	// Place order
	placedOrder, err := tc.placeStrangleOrder(order)
	if err != nil {
		tc.bot.logger.Printf("Failed to place order: %v", err)
		return
	}

	if placedOrder == nil {
		tc.bot.logger.Printf("ERROR: Broker returned nil order despite no error")
		return
	}

	tc.bot.logger.Printf("Order placed successfully: %d", placedOrder.Order.ID)

	// Create and save position
	position := tc.createPosition(order, expirationTime, placedOrder)
	if err := tc.bot.storage.AddPosition(position); err != nil {
		tc.bot.logger.Printf("Failed to save position: %v", err)
		return
	}

	tc.bot.logger.Printf("Position saved: ID=%s, LimitPrice=$%.2f, DTE=%d",
		position.ID, position.EntryLimitPrice, position.DTE)

	// Start order status polling
	go tc.bot.orderManager.PollOrderStatus(position.ID, placedOrder.Order.ID, true)
}

func (tc *TradingCycle) placeStrangleOrder(order *strategy.StrangleOrder) (*broker.OrderResponse, error) {
	tc.bot.logger.Printf("Placing strangle order for %d contracts...", order.Quantity)

	tickSize, err := tc.bot.broker.GetTickSize(order.Symbol)
	if err != nil {
		tc.bot.logger.Printf("Warning: Failed to get tick size for %s, using default 0.01: %v", 
			order.Symbol, err)
		tickSize = 0.01
	}

	px := math.Max(util.FloorToTick(order.Credit, tickSize), tickSize)
	tc.bot.logger.Printf("Using tick size %.4f for symbol %s, rounded price: $%.2f", 
		tickSize, order.Symbol, px)

	// Generate deterministic client-order ID with nonce to avoid duplicates
	canonicalString := fmt.Sprintf("entry-%s-%s-%.2f-%.2f-%d-%.2f-%s",
		order.Symbol, order.Expiration, order.PutStrike, order.CallStrike,
		order.Quantity, px, tc.bot.config.Broker.AccountID)

	hash := sha256.Sum256([]byte(canonicalString))
	base := "entry-" + hex.EncodeToString(hash[:])[:8]
	
	// Generate 4-hex nonce from crypto/rand
	nonceBytes := make([]byte, 2)
	if _, err := rand.Read(nonceBytes); err != nil {
		tc.bot.logger.Printf("Warning: Failed to generate nonce, using fallback: %v", err)
		// Fallback to time-based nonce if crypto/rand fails
		nonceBytes[0] = byte(time.Now().UnixNano() & 0xFF)
		nonceBytes[1] = byte((time.Now().UnixNano() >> 8) & 0xFF)
	}
	nonce := hex.EncodeToString(nonceBytes)
	clientOrderID := base + "-" + nonce

	return tc.bot.broker.PlaceStrangleOrder(
		order.Symbol,
		order.PutStrike,
		order.CallStrike,
		order.Expiration,
		order.Quantity,
		px,
		false,
		string(broker.DurationGTC),
		clientOrderID,
	)
}

func (tc *TradingCycle) createPosition(order *strategy.StrangleOrder, expirationTime time.Time, 
	placedOrder *broker.OrderResponse) *models.Position {
	
	positionID := uuid.New().String()

	position := models.NewPosition(
		positionID,
		order.Symbol,
		order.PutStrike,
		order.CallStrike,
		expirationTime,
		order.Quantity,
	)

	position.CreditReceived = order.Credit
	position.EntryLimitPrice = tc.computeEntryLimitPrice(order.Symbol, order.Credit)
	position.EntrySpot = order.SpotPrice
	position.DTE = position.CalculateDTE()
	position.EntryIV = tc.bot.strategy.GetCurrentIV()

	if placedOrder != nil {
		position.EntryOrderID = fmt.Sprintf("%d", placedOrder.Order.ID)
	} else {
		tc.bot.logger.Printf("Warning: Cannot set EntryOrderID due to nil placedOrder")
		position.EntryOrderID = ""
	}

	if err := position.TransitionState(models.StateSubmitted, models.ConditionOrderPlaced); err != nil {
		tc.bot.logger.Printf("Failed to set position state: %v", err)
	}

	return position
}

func (tc *TradingCycle) executeExit(position *models.Position, reason strategy.ExitReason) {
	tc.bot.logger.Printf("Executing exit for position %s: %s", shortID(position.ID), reason)

	if !tc.isPositionReadyForExit(position) {
		return
	}

	tc.logPositionClose(position)

	maxDebit := tc.calculateMaxDebit(position, reason)

	tickSize, err := tc.bot.broker.GetTickSize(position.Symbol)
	if err != nil {
		tc.bot.logger.Printf("Warning: Failed to get tick size for %s, using default 0.01: %v", 
			position.Symbol, err)
		tickSize = 0.01
	}

	if maxDebit <= 0 {
		tc.bot.logger.Printf("Warning: calculated maxDebit $%.2f is invalid for position %s, using minimum", 
			maxDebit, shortID(position.ID))
	}
	maxDebit = math.Max(maxDebit, tickSize)
	maxDebit = util.CeilToTick(maxDebit, tickSize)

	// Place close order
	closeOrder, err := tc.bot.retryClient.ClosePositionWithRetry(
		tc.bot.ctx,
		position,
		maxDebit,
	)

	if err != nil {
		tc.bot.logger.Printf("Failed to place close order for position %s: %v", shortID(position.ID), err)
		return
	}

	if closeOrder == nil {
		tc.bot.logger.Printf("ERROR: Close order placement succeeded but returned nil order for position %s", shortID(position.ID))
		return
	}

	// Update position
	position.ExitOrderID = fmt.Sprintf("%d", closeOrder.Order.ID)
	if err := tc.bot.storage.UpdatePosition(position); err != nil {
		tc.bot.logger.Printf("Failed to update position %s with exit order ID: %v", shortID(position.ID), err)
	}

	tc.bot.logger.Printf("Close order placed for position %s: order_id=%d, max_debit=$%.2f",
		shortID(position.ID), closeOrder.Order.ID, maxDebit)

	// Start order status polling
	go tc.bot.orderManager.PollOrderStatus(position.ID, closeOrder.Order.ID, false)
}

func (tc *TradingCycle) isPositionReadyForExit(position *models.Position) bool {
	currentState := position.GetCurrentState()
	if currentState == models.StateClosed {
		tc.bot.logger.Printf("Position %s is already closed, skipping duplicate close attempt", position.ID)
		return false
	}

	isManagementState := func(state models.PositionState) bool {
		return state == models.StateFirstDown ||
			state == models.StateSecondDown ||
			state == models.StateThirdDown ||
			state == models.StateFourthDown
	}

	if currentState == models.StateOpen || isManagementState(currentState) {
		return true
	}

	if currentState == models.StateAdjusting {
		if position.ExitOrderID == "" {
			tc.bot.logger.Printf("Position %s in Adjusting state with no active exit order, allowing re-attempt", 
				position.ID)
			return true
		}

		if position.ExitOrderID != "" {
			orderID, err := strconv.Atoi(position.ExitOrderID)
			if err != nil {
				tc.bot.logger.Printf("Position %s has invalid ExitOrderID %s: %v", 
					position.ID, position.ExitOrderID, err)
				return false
			}

			isTerminal, err := tc.bot.orderManager.IsOrderTerminal(tc.bot.ctx, orderID)
			if err != nil {
				tc.bot.logger.Printf("Failed to check order status for %s: %v, blocking re-attempt", 
					position.ExitOrderID, err)
				return false
			}

			if isTerminal {
				tc.bot.logger.Printf("Position %s prior close order %s is terminal, allowing re-attempt", 
					position.ID, position.ExitOrderID)
				position.ExitOrderID = ""
				position.ExitReason = ""
				if err := tc.bot.storage.UpdatePosition(position); err != nil {
					tc.bot.logger.Printf("Warning: Failed to clear terminal exit order ID: %v", err)
				}
				return true
			}
		}

		tc.bot.logger.Printf("Position %s in Adjusting state with active exit order %s, blocking duplicate",
			position.ID, position.ExitOrderID)
		return false
	}

	tc.bot.logger.Printf("Position %s is in state %s, not eligible for close", position.ID, currentState)
	return false
}

func (tc *TradingCycle) logPositionClose(position *models.Position) {
	tc.bot.logger.Printf("Closing position: %s %s Put %.0f / Call %.0f (State: %s)",
		position.Symbol, position.Expiration.Format("2006-01-02"),
		position.PutStrike, position.CallStrike, position.GetCurrentState())
}

func (tc *TradingCycle) calculateMaxDebit(position *models.Position, reason strategy.ExitReason) float64 {
	currentVal, cvErr := tc.bot.strategy.GetCurrentPositionValue(position)
	netCredit := position.GetNetCredit()
	absNetCredit := math.Abs(netCredit)

	pt := tc.bot.config.Strategy.Exit.ProfitTarget
	if pt < 0 || pt > 1 {
		tc.bot.logger.Printf("ERROR: Invalid ProfitTarget %.3f, using default 0.50", pt)
		pt = 0.50
	}

	sl := tc.bot.config.Strategy.Exit.StopLossPct
	if sl <= 1.0 {
		tc.bot.logger.Printf("ERROR: Invalid StopLossPct %.3f, using default 2.5", sl)
		sl = 2.5
	}

	switch reason {
	case strategy.ExitReasonProfitTarget:
		result := absNetCredit * (1.0 - pt)
		if result <= 0 {
			tc.bot.logger.Printf("ERROR: Calculated profit target debit (%.2f) is invalid", result)
			result = absNetCredit * 0.01
		}
		return result
		
	case strategy.ExitReasonTime:
		if cvErr == nil && position.Quantity != 0 {
			return currentVal / (float64(position.Quantity) * 100)
		}
		result := absNetCredit * (1.0 - pt)
		if result <= 0 {
			tc.bot.logger.Printf("ERROR: Fallback profit target debit (%.2f) is invalid", result)
			result = absNetCredit * 0.01
		}
		return result
		
	case strategy.ExitReasonStopLoss:
		if cvErr == nil && position.Quantity != 0 {
			return currentVal / (float64(position.Quantity) * 100)
		}
		result := absNetCredit * sl
		if result <= 0 {
			tc.bot.logger.Printf("ERROR: Calculated stop loss debit (%.2f) is invalid", result)
			result = absNetCredit * 2.0
		}
		return result
		
	default:
		return absNetCredit * 1.0
	}
}

func (tc *TradingCycle) checkAdjustmentsForPosition(position *models.Position) {
	// Placeholder for adjustment logic (Phase 2) - Football System
	// This would check if the position needs to be adjusted based on
	// market movement and the football system rules:
	// - First Down: Manage at 21 DTE or when tested
	// - Second Down: Roll for additional credit 
	// - Third Down: Roll again if beneficial
	// - Fourth Down: Close position (punt)
	if tc.bot.config.Strategy.Adjustments.EnableAdjustmentStub {
		tc.bot.logger.Printf("Football System adjustment check for position %s not yet implemented", shortID(position.ID))
	}
}

// computeEntryLimitPrice calculates the entry limit price using the correct tick size for the symbol
func (tc *TradingCycle) computeEntryLimitPrice(symbol string, credit float64) float64 {
	tickSize, err := tc.bot.broker.GetTickSize(symbol)
	if err != nil {
		tc.bot.logger.Printf("Warning: Failed to get tick size for %s, using default 0.01: %v", symbol, err)
		tickSize = 0.01
	}
	return math.Max(util.FloorToTick(credit, tickSize), tickSize)
}