package orders

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
)

type Config struct {
	PollInterval time.Duration
	Timeout      time.Duration
}

var DefaultConfig = Config{
	PollInterval: 5 * time.Second,
	Timeout:      5 * time.Minute,
}

type Manager struct {
	broker  broker.Broker
	storage storage.StorageInterface
	logger  *log.Logger
	stop    <-chan struct{}
	config  Config
}

func NewManager(
	broker broker.Broker,
	storage storage.StorageInterface,
	logger *log.Logger,
	stop <-chan struct{},
	config ...Config,
) *Manager {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	return &Manager{
		broker:  broker,
		storage: storage,
		logger:  logger,
		stop:    stop,
		config:  cfg,
	}
}

func (m *Manager) PollOrderStatus(positionID string, orderID int) {
	m.logger.Printf("Starting order status polling for position %s, order %d", positionID, orderID)

	ctx, cancel := context.WithTimeout(context.Background(), m.config.Timeout)
	defer cancel()

	ticker := time.NewTicker(m.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Printf("Order polling timeout for position %s", positionID)
			m.handleOrderTimeout(positionID)
			return
		case <-m.stop:
			m.logger.Printf("Shutdown signal received during order polling for position %s", positionID)
			return
		case <-ticker.C:
			orderStatus, err := m.broker.GetOrderStatus(orderID)
			if err != nil {
				m.logger.Printf("Error checking order status for %s: %v", positionID, err)
				continue
			}

			m.logger.Printf("Order %d status: %s", orderID, orderStatus.Order.Status)

			switch orderStatus.Order.Status {
			case "filled":
				m.logger.Printf("Order filled for position %s", positionID)
				m.handleOrderFilled(positionID)
				return
			case "canceled", "rejected":
				m.logger.Printf("Order failed for position %s: %s", positionID, orderStatus.Order.Status)
				m.handleOrderFailed(positionID, orderStatus.Order.Status)
				return
			case "pending", "open", "partial":
				continue
			default:
				m.logger.Printf("Unknown order status for position %s: %s", positionID, orderStatus.Order.Status)
			}
		}
	}
}

func (m *Manager) handleOrderFilled(positionID string) {
	position := m.storage.GetCurrentPosition()
	if position == nil || position.ID != positionID {
		m.logger.Printf("Position %s not found or mismatched", positionID)
		return
	}

	if err := position.TransitionState(models.StateOpen, "order_filled"); err != nil {
		m.logger.Printf("Failed to transition position %s to open: %v", positionID, err)
		return
	}

	if err := m.storage.SetCurrentPosition(position); err != nil {
		m.logger.Printf("Failed to save position %s after fill: %v", positionID, err)
		return
	}

	m.logger.Printf("Position %s successfully transitioned to open state", positionID)
}

func (m *Manager) handleOrderFailed(positionID string, reason string) {
	position := m.storage.GetCurrentPosition()
	if position == nil || position.ID != positionID {
		m.logger.Printf("Position %s not found or mismatched", positionID)
		return
	}

	if err := position.TransitionState(models.StateError, fmt.Sprintf("order_%s", reason)); err != nil {
		m.logger.Printf("Failed to transition position %s to error: %v", positionID, err)
		return
	}

	if err := m.storage.SetCurrentPosition(position); err != nil {
		m.logger.Printf("Failed to save position %s after failure: %v", positionID, err)
		return
	}

	m.logger.Printf("Position %s marked as error due to order failure: %s", positionID, reason)
}

func (m *Manager) handleOrderTimeout(positionID string) {
	position := m.storage.GetCurrentPosition()
	if position == nil || position.ID != positionID {
		m.logger.Printf("Position %s not found or mismatched", positionID)
		return
	}

	if err := position.TransitionState(models.StateError, "order_timeout"); err != nil {
		m.logger.Printf("Failed to transition position %s to error: %v", positionID, err)
		return
	}

	if err := m.storage.SetCurrentPosition(position); err != nil {
		m.logger.Printf("Failed to save position %s after timeout: %v", positionID, err)
		return
	}

	m.logger.Printf("Position %s marked as error due to order timeout", positionID)
}
