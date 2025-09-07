// Package orders provides order management functionality for the trading bot.
package orders

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/eddiefleurent/scranton_strangler/internal/strategy"
)

// State transition reasons
const (
	exitConditions = "exit_conditions"
	hardStop       = "hard_stop"
	forceClose     = "force_close"
)

// Config contains configuration for the order manager.
type Config struct {
	PollInterval time.Duration
	Timeout      time.Duration
}

// DefaultConfig is the default configuration for the order manager.
var DefaultConfig = Config{
	PollInterval: 5 * time.Second,
	Timeout:      5 * time.Minute,
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
			statusCtx, statusCancel := context.WithTimeout(ctx, 5*time.Second)
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

			m.logger.Printf("Order %d status: %s", orderID, orderStatus.Order.Status)

			switch orderStatus.Order.Status {
			case "filled":
				m.logger.Printf("Order filled for position %s", positionID)
				m.handleOrderFilled(positionID, isEntryOrder)
				return
			case "canceled", "cancelled", "rejected", "expired":
				m.logger.Printf("Order failed for position %s: %s", positionID, orderStatus.Order.Status)
				m.handleOrderFailed(positionID, orderID, orderStatus.Order.Status)
				return
			case "pending", "open", "partial", "partially_filled":
				continue
			default:
				m.logger.Printf("Unknown order status for position %s: %s", positionID, orderStatus.Order.Status)
				continue
			}
		}
	}
}

func (m *Manager) handleOrderFilled(positionID string, isEntryOrder bool) {
	position := m.storage.GetCurrentPosition()
	if position == nil || position.ID != positionID {
		m.logger.Printf("Position %s not found or mismatched", positionID)
		return
	}

	var targetState models.PositionState
	var transitionReason string

	if isEntryOrder {
		targetState = models.StateOpen
		transitionReason = "order_filled"
	} else {
		// For exit orders, transition to closed state with proper reason mapping
		targetState = models.StateClosed
		transitionReason = m.exitConditionFromReason(position.ExitReason)

		// Parse exit reason from stored value
		exitReason := strategy.ExitReason(position.ExitReason)
		m.logger.Printf("Exit order filled for position %s with reason: %s", positionID, exitReason)

		// Calculate final P&L (simplified version)
		// Note: This is a simplified calculation. The full P&L calculation
		// should be done by the bot with access to strategy methods
		totalCredit := position.CreditReceived * float64(position.Quantity) * 100
		if position.CurrentPnL != 0 {
			// Use current P&L if available
			m.logger.Printf("Position %s exit order filled. Final P&L: $%.2f", positionID, position.CurrentPnL)
		} else {
			// Fallback to credit received
			m.logger.Printf("Position %s exit order filled. Estimated P&L: $%.2f", positionID, totalCredit)
		}
	}

	if err := position.TransitionState(targetState, transitionReason); err != nil {
		m.logger.Printf("Failed to transition position %s to %s: %v", positionID, targetState, err)
		return
	}

	if err := m.storage.SetCurrentPosition(position); err != nil {
		m.logger.Printf("Failed to save position %s after fill: %v", positionID, err)
		return
	}

	m.logger.Printf("Position %s successfully transitioned to %s state", positionID, targetState)
}

func (m *Manager) handleOrderFailed(positionID string, orderID int, reason string) {
	position := m.storage.GetCurrentPosition()
	if position == nil || position.ID != positionID {
		m.logger.Printf("Position %s not found or mismatched", positionID)
		return
	}

	// Check if this is an exit order failure - verify the orderID matches
	isExitOrder := position.ExitOrderID != "" &&
		position.ExitOrderID == fmt.Sprintf("%d", orderID)

	if isExitOrder {
		m.logger.Printf("Exit order failed for position %s: %s, marking position as error and clearing exit order", positionID, reason)
		// For exit order failures, mark position as error and clear the exit order ID
		position.ExitOrderID = ""
		position.ExitReason = ""
		if err := position.TransitionState(models.StateError, "adjustment_failed"); err != nil {
			m.logger.Printf("Failed to transition position %s to error: %v", positionID, err)
		}
	} else {
		if err := position.TransitionState(models.StateError, "order_failed"); err != nil {
			m.logger.Printf("Failed to transition position %s to error: %v", positionID, err)
			return
		}
	}

	if err := m.storage.SetCurrentPosition(position); err != nil {
		m.logger.Printf("Failed to save position %s after failure: %v", positionID, err)
		return
	}

	if isExitOrder {
		m.logger.Printf("Position %s reverted to active state due to exit order failure: %s", positionID, reason)
	} else {
		m.logger.Printf("Position %s marked as error due to order failure: %s", positionID, reason)
	}
}

func (m *Manager) handleOrderTimeout(positionID string) {
	position := m.storage.GetCurrentPosition()
	if position == nil || position.ID != positionID {
		m.logger.Printf("Position %s not found or mismatched", positionID)
		return
	}

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
	if currentState == models.StateSubmitted && !isExitOrder {
		if err := position.TransitionState(models.StateClosed, "order_timeout"); err != nil {
			m.logger.Printf("Failed to close position %s on entry timeout: %v", positionID, err)
			return
		}
	} else {
		// For exit timeouts, use the state-specific transition reason
		if err := position.TransitionState(models.StateClosed, transitionReason); err != nil {
			// If direct transition to closed fails, transition to error state
			m.logger.Printf("Cannot transition %s from %s to closed: %v. Marking as error.",
				positionID, currentState, err)
			if errFallback := position.TransitionState(models.StateError, "order_timeout"); errFallback != nil {
				m.logger.Printf("Failed to transition position %s to error: %v", positionID, errFallback)
				return
			}
		}
	}

	if err := m.storage.SetCurrentPosition(position); err != nil {
		m.logger.Printf("Failed to save position %s after timeout: %v", positionID, err)
		return
	}

	m.logger.Printf("Position %s closed due to order timeout", positionID)
}

// exitConditionFromReason maps stored exit reasons to canonical transition reasons
func (m *Manager) exitConditionFromReason(exitReason string) string {
	switch exitReason {
	case "profit_target", "time", "manual":
		return exitConditions
	case "escalate":
		return "emergency_exit"
	case "stop_loss", "error":
		return hardStop
	default:
		return exitConditions // Default fallback
	}
}

// timeoutTransitionReason returns the appropriate transition reason for order timeouts
// based on the current state, ensuring it matches ValidTransitions
func (m *Manager) timeoutTransitionReason(currentState models.PositionState) string {
	switch currentState {
	case models.StateAdjusting:
		return hardStop // StateAdjusting -> StateClosed requires "hard_stop"
	case models.StateRolling:
		return forceClose // StateRolling -> StateClosed requires "force_close"
	case models.StateFirstDown, models.StateSecondDown:
		return exitConditions // These states allow "exit_conditions" -> StateClosed
	case models.StateThirdDown:
		return hardStop // StateThirdDown -> StateClosed requires "hard_stop"
	case models.StateFourthDown:
		return "emergency_exit" // StateFourthDown -> StateClosed requires "emergency_exit"
	case models.StateError:
		return forceClose // StateError -> StateClosed requires "force_close"
	default:
		return forceClose // Default fallback for any other states
	}
}
