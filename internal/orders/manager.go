// Package orders provides order management functionality for the trading bot.
package orders

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
)


// Config contains configuration for the order manager.
type Config struct {
	PollInterval time.Duration
	Timeout      time.Duration
	CallTimeout  time.Duration
}

// DefaultConfig is the default configuration for the order manager.
var DefaultConfig = Config{
	PollInterval: 5 * time.Second,
	Timeout:      5 * time.Minute,
	CallTimeout:  5 * time.Second,
}

// Manager handles order execution and status polling.
type Manager struct {
	broker  broker.Broker
	storage storage.Interface
	logger  *log.Logger
	stop    <-chan struct{}
	config  Config
}

// NewManager creates a new order manager instance.
func NewManager(
	broker broker.Broker,
	storage storage.Interface,
	logger *log.Logger,
	stop <-chan struct{},
	config ...Config,
) *Manager {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	// Guard against nil logger
	if logger == nil {
		logger = log.New(os.Stderr, "orders: ", log.LstdFlags)
	}

	// Validate and clamp config values
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultConfig.PollInterval
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultConfig.Timeout
	}
	if cfg.CallTimeout <= 0 {
		cfg.CallTimeout = DefaultConfig.CallTimeout
	}

	// Validate required dependencies (fail fast to avoid later panics)
	if broker == nil {
		panic("orders.NewManager: broker must not be nil")
	}
	if storage == nil {
		panic("orders.NewManager: storage must not be nil")
	}

	return &Manager{
		broker:  broker,
		storage: storage,
		logger:  logger,
		stop:    stop,
		config:  cfg,
	}
}

// PollOrderStatus polls the status of an order until it's filled or fails.
func (m *Manager) PollOrderStatus(positionID string, orderID int, isEntryOrder bool) {
	m.logger.Printf("Starting order status polling for position %s, order %d", positionID, orderID)

	ctx, cancel := context.WithTimeout(context.Background(), m.config.Timeout)
	defer cancel()

	ticker := time.NewTicker(m.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Printf("Order polling timeout for position %s, order %d", positionID, orderID)
			m.handleOrderTimeout(positionID)
			return
		case <-m.stop:
			m.logger.Printf("Shutdown signal received during order polling for position %s", positionID)
			return
		case <-ticker.C:
			// Create a child context with short timeout for the GetOrderStatus call
			statusCtx, statusCancel := context.WithTimeout(ctx, m.config.CallTimeout)
			orderStatus, err := m.broker.GetOrderStatusCtx(statusCtx, orderID)
			statusCancel() // Explicitly cancel the context after the call

			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					m.logger.Printf("GetOrderStatus timeout for position %s, order %d", positionID, orderID)
					continue // Continue the loop on timeout so ticker keeps running
				}
				m.logger.Printf("Error checking order status for %s: %v", positionID, err)
				continue
			}

			if orderStatus == nil {
				m.logger.Printf("Nil order status for %d", orderID)
				continue
			}

			if orderStatus.Order.ID == 0 {
				m.logger.Printf("Order payload missing for %d", orderID)
				continue
			}

			if orderStatus.Order.Status == "" {
				m.logger.Printf("Order %d has empty status field, cannot determine status", orderID)
				continue
			}

			status := strings.ToLower(orderStatus.Order.Status)
			
			// Check if order is completely filled by comparing executed vs requested quantity
			// This handles cases where status is "partial" but all contracts have actually filled
			isCompletelyFilled := m.isOrderCompletelyFilled(orderStatus)
			
			m.logger.Printf("Order %d status: %s, exec_qty: %.0f, total_qty: %.0f, remaining: %.0f, completely_filled: %t", 
				orderID, status, orderStatus.Order.ExecQuantity, orderStatus.Order.Quantity, 
				orderStatus.Order.RemainingQuantity, isCompletelyFilled)

			// Handle completely filled orders (regardless of status string)
			if isCompletelyFilled {
				m.logger.Printf("Order completely filled for position %s", positionID)
				m.handleOrderFilled(positionID, isEntryOrder)
				return
			}

			// Handle explicitly failed orders
			switch status {
			case "canceled", "cancelled", "rejected", "expired":
				m.logger.Printf("Order failed for position %s: %s", positionID, orderStatus.Order.Status)
				m.handleOrderFailed(positionID, orderID, orderStatus.Order.Status)
				return
			case "pending", "open", "partial", "partially_filled", "filled":
				// Continue polling for non-terminal states
				// Note: "filled" is included here because isCompletelyFilled check above handles true fills
				continue
			default:
				m.logger.Printf("Unknown order status for position %s: %s", positionID, orderStatus.Order.Status)
				continue
			}
		}
	}
}

func (m *Manager) handleOrderFilled(positionID string, isEntryOrder bool) {
	// Try to get position by ID first (for multiple positions support)
	position, found := m.storage.GetPositionByID(positionID)
	if !found {
		m.logger.Printf("Position %s not found", positionID)
		return
	}

	var targetState models.PositionState
	var transitionReason string

	if isEntryOrder {
		targetState = models.StateOpen
		transitionReason = "order_filled"

		if err := position.TransitionState(targetState, transitionReason); err != nil {
			m.logger.Printf("Failed to transition position %s to %s: %v", positionID, targetState, err)
			return
		}

		if err := m.storage.UpdatePosition(&position); err != nil {
			m.logger.Printf("Failed to save position %s after fill: %v", positionID, err)
			return
		}

		m.logger.Printf("Position %s successfully transitioned to %s state", positionID, targetState)
	} else {
		// For exit orders, use ClosePosition API for atomic state transition and persistence
		transitionReason = m.exitConditionFromReason(position.ExitReason)

		// Parse exit reason from stored value
		exitReason := strategy.ExitReason(position.ExitReason)
		m.logger.Printf("Exit order filled for position %s with reason: %s", positionID, exitReason)

		// Calculate final P&L (simplified version)
		// Note: This is a simplified calculation. The full P&L calculation
		// should be done by the bot with access to strategy methods
		finalPnL := position.CurrentPnL
		if finalPnL == 0 {
			// Fallback to credit received if CurrentPnL is zero
			finalPnL = position.CreditReceived * float64(position.Quantity) * 100
		}

		// Close position using position ID
		if err := m.storage.ClosePositionByID(positionID, finalPnL, transitionReason); err != nil {
			m.logger.Printf("Failed to close position %s: %v", positionID, err)
			return
		}

		m.logger.Printf("Position %s successfully closed. Final P&L: $%.2f", positionID, finalPnL)
	}
}

func (m *Manager) handleOrderFailed(positionID string, orderID int, reason string) {
	// Try to get position by ID first (for multiple positions support)
	position, found := m.storage.GetPositionByID(positionID)
	if !found {
		m.logger.Printf("Position %s not found", positionID)
		return
	}

	// Check if this is an exit order failure - verify the orderID matches
	isExitOrder := position.ExitOrderID != "" &&
		position.ExitOrderID == fmt.Sprintf("%d", orderID)

	if isExitOrder {
		m.logger.Printf("Exit order failed for position %s: %s, keeping position active and clearing exit order", positionID, reason)
		// For exit order failures, keep position state unchanged and clear the exit order ID
		position.ExitOrderID = ""
		position.ExitReason = ""
	} else {
		if err := position.TransitionState(models.StateError, "order_failed"); err != nil {
			m.logger.Printf("Failed to transition position %s to error: %v", positionID, err)
			return
		}
	}

	if err := m.storage.UpdatePosition(&position); err != nil {
		m.logger.Printf("Failed to save position %s after failure: %v", positionID, err)
		return
	}

	if isExitOrder {
		m.logger.Printf("Position %s kept active due to exit order failure: %s", positionID, reason)
	} else {
		m.logger.Printf("Position %s marked as error due to order failure: %s", positionID, reason)
	}
}

func (m *Manager) handleOrderTimeout(positionID string) {
	position, found := m.storage.GetPositionByID(positionID)
	if !found {
		m.logger.Printf("Position %s not found", positionID)
		return
	}

	// Before closing position, check if broker actually has open positions matching this trade
	// This prevents closing positions that actually filled but we lost track due to polling timeout
	m.logger.Printf("Order timeout for position %s - verifying broker state before closing", positionID)
	
	// Bound the broker probe by CallTimeout to avoid indefinite hangs
	posCtx, cancel := context.WithTimeout(context.Background(), m.config.CallTimeout)
	defer cancel()
	brokerPositions, err := m.getPositionsWithTimeout(posCtx)
	if err != nil {
		m.logger.Printf("Failed to get broker positions during timeout handling: %v", err)
		// Continue with original timeout logic as fallback
	} else {
		// Check if this position actually exists in the broker
		isOpenInBroker := m.isPositionOpenInBroker(&position, brokerPositions)
		if isOpenInBroker {
			m.logger.Printf("Position %s order timed out but position exists in broker - transitioning to open state", positionID)
			
			// The order actually filled! Transition to open state instead of closing
			if err := position.TransitionState(models.StateOpen, "order_filled"); err != nil {
				m.logger.Printf("Failed to transition timed-out position %s to open: %v", positionID, err)
				// Continue with timeout closure as fallback
			} else {
				if err := m.storage.UpdatePosition(&position); err != nil {
					m.logger.Printf("Failed to save recovered position %s: %v", positionID, err)
				} else {
					m.logger.Printf("Successfully recovered position %s from timeout - position was actually filled", positionID)
					return // Exit early - position recovered
				}
			}
		}
	}

	// Original timeout handling - only reached if broker check failed or position not found in broker
	m.logger.Printf("Proceeding with timeout closure for position %s", positionID)

	// Detect entry vs exit order timeout
	isExitOrder := position.ExitOrderID != "" && position.GetCurrentState() != models.StateSubmitted

	var transitionReason string
	if isExitOrder {
		// For exit order timeouts, determine transition reason based on current state
		transitionReason = m.timeoutTransitionReason(position.GetCurrentState())
	} else {
		// For entry order timeouts, use the standard timeout reason
		transitionReason = "order_timeout"
	}

	// For entry timeouts (from StateSubmitted), transition to StateClosed with "order_timeout"
	// For other states, check if the transition is valid
	currentState := position.GetCurrentState()

	var finalPnL float64
	var closeReason string

	if currentState == models.StateSubmitted && !isExitOrder {
		// Entry order timeout - position never opened, so P&L is 0
		finalPnL = 0
		closeReason = "order_timeout"

		// Prefer ID-aware API; fallback for back-compat
		if err := m.storage.ClosePositionByID(positionID, finalPnL, closeReason); err != nil {
			m.logger.Printf("Failed to close position %s on entry timeout: %v", positionID, err)
			return
		}

		m.logger.Printf("Position %s closed due to entry order timeout. Final P&L: $%.2f", positionID, finalPnL)
	} else {
		// Exit timeout: compute P&L (prefer CurrentPnL) and finalize via storage
		finalPnL = position.CurrentPnL
		if finalPnL == 0 {
			finalPnL = position.CreditReceived * float64(position.Quantity) * 100
		}
		closeReason = transitionReason

		if err := m.storage.ClosePositionByID(positionID, finalPnL, closeReason); err != nil {
			m.logger.Printf("Failed to close position %s on exit timeout: %v", positionID, err)
			return
		}

		m.logger.Printf("Position %s closed due to exit order timeout. Final P&L: $%.2f", positionID, finalPnL)
	}
}

// exitConditionFromReason maps stored exit reasons to canonical transition reasons
func (m *Manager) exitConditionFromReason(exitReason string) string {
	switch exitReason {
	case "profit_target", "time", "manual":
		return models.ConditionExitConditions
	case "escalate":
		return models.ConditionEmergencyExit
	case "stop_loss", "error":
		return models.ConditionHardStop
	default:
		return models.ConditionExitConditions // Default fallback
	}
}

// timeoutTransitionReason returns the appropriate transition reason for order timeouts
// based on the current state, ensuring it matches ValidTransitions
func (m *Manager) timeoutTransitionReason(currentState models.PositionState) string {
	switch currentState {
	case models.StateAdjusting:
		return models.ConditionHardStop // StateAdjusting -> StateClosed requires "hard_stop"
	case models.StateRolling:
		return models.ConditionForceClose // StateRolling -> StateClosed requires "force_close"
	case models.StateFirstDown, models.StateSecondDown:
		return models.ConditionExitConditions // These states allow "exit_conditions" -> StateClosed
	case models.StateThirdDown:
		return models.ConditionHardStop // StateThirdDown -> StateClosed requires "hard_stop"
	case models.StateFourthDown:
		return models.ConditionEmergencyExit // StateFourthDown -> StateClosed requires "emergency_exit"
	case models.StateError:
		return models.ConditionForceClose // StateError -> StateClosed requires "force_close"
	default:
		return models.ConditionForceClose // Default fallback for any other states
	}
}

// IsOrderTerminal checks if an order has reached a terminal state (filled, canceled, rejected)
func (m *Manager) IsOrderTerminal(ctx context.Context, orderID int) (bool, error) {
	// Create a child context with short timeout for the order status check
	statusCtx, cancel := context.WithTimeout(ctx, m.config.CallTimeout)
	defer cancel()

	orderStatus, err := m.broker.GetOrderStatusCtx(statusCtx, orderID)
	if err != nil {
		return false, fmt.Errorf("failed to get order status: %w", err)
	}

	if orderStatus == nil || orderStatus.Order.ID == 0 {
		return false, fmt.Errorf("invalid order status response")
	}

	status := strings.ToLower(orderStatus.Order.Status)
	// Terminal states: filled, canceled, cancelled, rejected, expired
	switch status {
	case "filled", "canceled", "cancelled", "rejected", "expired":
		return true, nil
	default:
		return false, nil
	}
}

// isOrderCompletelyFilled determines if an order is completely filled by checking
// executed quantity against total quantity, accounting for floating point precision
func (m *Manager) isOrderCompletelyFilled(orderStatus *broker.OrderResponse) bool {
	if orderStatus == nil {
		return false
	}
	
	order := orderStatus.Order
	
	// If explicitly marked as filled, it's definitely complete
	status := strings.ToLower(order.Status)
	if status == "filled" {
		return true
	}
	
	// For other statuses, check if exec_quantity >= quantity
	// Use small epsilon for floating point comparison
	const epsilon = 1e-6
	
	// Handle zero quantity orders (shouldn't happen but be defensive)
	if order.Quantity <= epsilon {
		return false
	}
	
	// Order is completely filled if executed quantity equals or exceeds requested quantity
	isComplete := order.ExecQuantity >= (order.Quantity - epsilon)
	
	// Additional validation: remaining quantity should be zero (or very close to zero)
	hasZeroRemaining := order.RemainingQuantity <= epsilon
	
	// Critical fix: Don't consider an order filled if nothing was executed (rejected orders)
	nothingExecuted := order.ExecQuantity <= epsilon
	
	m.logger.Printf("Fill check - ExecQty: %.6f, TotalQty: %.6f, Remaining: %.6f, Complete: %t, ZeroRemaining: %t, NothingExecuted: %t",
		order.ExecQuantity, order.Quantity, order.RemainingQuantity, isComplete, hasZeroRemaining, nothingExecuted)
	
	// Order is complete only if:
	// 1. Executed quantity >= requested quantity, OR
	// 2. Remaining quantity is zero AND something was actually executed (not a rejected order)
	return isComplete || (hasZeroRemaining && !nothingExecuted)
}

// isPositionOpenInBroker checks if a stored position still exists in broker positions
func (m *Manager) isPositionOpenInBroker(position *models.Position, brokerPositions []broker.PositionItem) bool {
	wantExp := position.Expiration.Format("2006-01-02")
	hasCallStrike := false
	hasPutStrike := false

	for _, bp := range brokerPositions {
		// Only consider this underlying - use strict equality to avoid false matches
		brokerUnderlying := m.extractUnderlyingFromSymbol(bp.Symbol)
		if brokerUnderlying != position.Symbol {
			continue
		}
		// Match expiration
		exp := m.extractExpirationFromSymbol(bp.Symbol)
		if exp != wantExp {
			continue
		}
		// Parse option leg
		strike, optType, err := m.parseOptionSymbol(bp.Symbol)
		if err != nil || bp.Quantity == 0 {
			continue
		}
		if optType == "C" && math.Abs(strike-position.CallStrike) < 0.01 {
			hasCallStrike = true
		}
		if optType == "P" && math.Abs(strike-position.PutStrike) < 0.01 {
			hasPutStrike = true
		}
	}
	return hasCallStrike && hasPutStrike
}

// extractExpirationFromSymbol extracts expiration date from OPRA option symbol
// Format: TICKER[YYMMDD][C/P][STRIKE*1000 padded to 8 digits]
// Example: SPY240315C00610000 -> "2024-03-15"
func (m *Manager) extractExpirationFromSymbol(symbol string) string {
	// Find a 'C' or 'P' followed by exactly 8 digits (strike*1000),
	// then take the preceding 6 digits as YYMMDD.
	for i := len(symbol) - 9; i >= 6; i-- {
		ch := symbol[i]
		if ch != 'C' && ch != 'P' {
			continue
		}
		if i+9 > len(symbol) {
			continue
		}
		strikeStr := symbol[i+1 : i+9]
		allDigits := true
		for _, r := range strikeStr {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			continue
		}
		dateStart := i - 6
		if dateStart < 0 {
			continue
		}
		dateStr := symbol[dateStart:i]
		for _, r := range dateStr {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			continue
		}
		year := "20" + dateStr[0:2]
		month := dateStr[2:4]
		day := dateStr[4:6]
		return year + "-" + month + "-" + day
	}
	return ""
}

// extractUnderlyingFromSymbol extracts the underlying symbol from an OPRA option symbol
// by finding the C/P option type followed by 8-digit strike, then parsing the underlying
func (m *Manager) extractUnderlyingFromSymbol(symbol string) string {
	// Find a 'C' or 'P' followed by exactly 8 digits (strike*1000)
	for i := len(symbol) - 9; i >= 6; i-- {
		ch := symbol[i]
		if ch != 'C' && ch != 'P' {
			continue
		}
		if i+9 > len(symbol) {
			continue
		}
		strikeStr := symbol[i+1 : i+9]
		allDigits := true
		for _, r := range strikeStr {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			continue
		}
		// Found valid option type and strike, preceding 6 chars should be date
		dateStart := i - 6
		if dateStart < 0 {
			continue
		}
		dateStr := symbol[dateStart:i]
		for _, r := range dateStr {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			continue
		}
		// Everything before the date is the underlying
		return symbol[:dateStart]
	}
	return ""
}

// parseOptionSymbol parses an OPRA format option symbol to extract strike and type
// Format: TICKER[YYMMDD][C/P][STRIKE*1000 padded to 8 digits]
// Example: SPY240315C00610000 -> strike=610.00, type="C"
func (m *Manager) parseOptionSymbol(symbol string) (float64, string, error) {
	if len(symbol) < 9 {
		return 0, "", fmt.Errorf("option symbol too short: %s", symbol)
	}
	
	// Find the last C or P that is followed by exactly 8 digits
	// This is more robust than assuming fixed positions
	for i := len(symbol) - 9; i >= 0; i-- {
		if symbol[i] == 'C' || symbol[i] == 'P' {
			// Check if the remaining characters after C/P are exactly 8 digits
			if i+9 > len(symbol) {
				continue // Not enough characters for 8-digit strike
			}
			
			strikeStr := symbol[i+1 : i+9]
			
			// Validate that all 8 characters are digits
			isAllDigits := true
			for _, char := range strikeStr {
				if char < '0' || char > '9' {
					isAllDigits = false
					break
				}
			}
			
			if !isAllDigits {
				continue // Not 8 consecutive digits, keep looking
			}
			
			// Parse the 8-digit strike
			strikeInt, err := strconv.ParseInt(strikeStr, 10, 64)
			if err != nil {
				return 0, "", fmt.Errorf("failed to parse strike from symbol %s: %w", symbol, err)
			}
			
			// Convert to float (divide by 1000 per OPRA format)
			strike := float64(strikeInt) / 1000.0
			optionType := string(symbol[i])
			
			return strike, optionType, nil
		}
	}
	
	return 0, "", fmt.Errorf("no valid C/P followed by 8-digit strike found in symbol: %s", symbol)
}

// getPositionsWithTimeout wraps Broker.GetPositionsCtx() with a context timeout.
func (m *Manager) getPositionsWithTimeout(ctx context.Context) ([]broker.PositionItem, error) {
	return m.broker.GetPositionsCtx(ctx)
}
