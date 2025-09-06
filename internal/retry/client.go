package retry

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
)

type Config struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Timeout        time.Duration
}

var DefaultConfig = Config{
	MaxRetries:     3,
	InitialBackoff: 1 * time.Second,
	MaxBackoff:     30 * time.Second,
	Timeout:        2 * time.Minute,
}

type Client struct {
	broker broker.Broker
	logger *log.Logger
	config Config
}

func NewClient(broker broker.Broker, logger *log.Logger, config ...Config) *Client {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	return &Client{
		broker: broker,
		logger: logger,
		config: cfg,
	}
}

func (c *Client) ClosePositionWithRetry(
	ctx context.Context,
	position *models.Position,
	maxDebit float64,
) (*broker.OrderResponse, error) {
	closeCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	var lastErr error
	backoff := c.config.InitialBackoff

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		select {
		case <-closeCtx.Done():
			return nil, fmt.Errorf("close operation timed out after %v: %w", c.config.Timeout, closeCtx.Err())
		default:
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("operation canceled: %w", ctx.Err())
		}

		c.logger.Printf("Close attempt %d/%d for position %s", attempt+1, c.config.MaxRetries+1, position.ID)

		closeOrder, err := c.broker.CloseStranglePosition(
			position.Symbol,
			position.PutStrike,
			position.CallStrike,
			position.Expiration.Format("2006-01-02"),
			position.Quantity,
			maxDebit,
		)

		if err == nil {
			c.logger.Printf("Close order placed successfully on attempt %d: %d", attempt+1, closeOrder.Order.ID)
			return closeOrder, nil
		}

		lastErr = err
		c.logger.Printf("Close attempt %d failed: %v", attempt+1, err)

		if c.isTransientError(err) && attempt < c.config.MaxRetries {
			c.logger.Printf("Transient error detected, retrying in %v", backoff)
			select {
			case <-time.After(backoff):
				backoff = c.calculateNextBackoff(backoff)
			case <-closeCtx.Done():
				return nil, fmt.Errorf("close operation timed out during backoff: %w", closeCtx.Err())
			case <-ctx.Done():
				return nil, fmt.Errorf("operation canceled during backoff: %w", ctx.Err())
			}
		} else {
			break
		}
	}

	return nil, fmt.Errorf("failed to close position after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

func (c *Client) calculateNextBackoff(currentBackoff time.Duration) time.Duration {
	backoff := time.Duration(float64(currentBackoff) * 1.5)
	if backoff > c.config.MaxBackoff {
		backoff = c.config.MaxBackoff
	}

	maxJitter := int64(backoff / 4)
	if maxJitter > 0 {
		jitterVal, err := rand.Int(rand.Reader, big.NewInt(maxJitter))
		if err != nil {
			log.Printf("Failed to generate jitter: %v", err)
		} else {
			jitter := time.Duration(jitterVal.Int64())
			backoff += jitter
		}
	}

	return backoff
}

func (c *Client) isTransientError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	transientPatterns := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"server error",
		"rate limit",
		"429", // HTTP 429 Too Many Requests
		"502", // HTTP 502 Bad Gateway
		"503", // HTTP 503 Service Unavailable
		"504", // HTTP 504 Gateway Timeout
		"network",
		"dns",
		"tcp",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}
