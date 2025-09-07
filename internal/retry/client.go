// Package retry provides retry logic for broker operations with exponential backoff.
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

// Config contains retry configuration parameters.
type Config struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Timeout        time.Duration
}

// DefaultConfig provides sensible defaults for retry operations.
var DefaultConfig = Config{
	MaxRetries:     3,
	InitialBackoff: 1 * time.Second,
	MaxBackoff:     30 * time.Second,
	Timeout:        2 * time.Minute,
}

// Client wraps a broker with retry logic for operations.
type Client struct {
	broker broker.Broker
	logger *log.Logger
	config Config
}

// NewClient creates a new retry client with the given broker and optional config.
func NewClient(broker broker.Broker, logger *log.Logger, config ...Config) *Client {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	// Default nil logger to log.Default()
	if logger == nil {
		logger = log.Default()
	}

	// Validate and sanitize config fields
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = DefaultConfig.MaxRetries
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = DefaultConfig.InitialBackoff
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = DefaultConfig.MaxBackoff
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultConfig.Timeout
	}
	if cfg.MaxBackoff < cfg.InitialBackoff {
		cfg.MaxBackoff = cfg.InitialBackoff
	}

	return &Client{
		broker: broker,
		logger: logger,
		config: cfg,
	}
}

// ClosePositionWithRetry attempts to close a position with retry logic and exponential backoff.
func (c *Client) ClosePositionWithRetry(
	ctx context.Context,
	position *models.Position,
	maxDebit float64,
) (*broker.OrderResponse, error) {
	// Add nil guard for position argument
	if position == nil {
		c.logger.Printf("ClosePositionWithRetry called with nil position")
		return nil, fmt.Errorf("nil position provided to ClosePositionWithRetry")
	}

	closeCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	// Generate a stable client-order ID for deduplication across retry attempts
	// Format: close-{positionID}-{expiration}-{timestamp}
	clientOrderID := fmt.Sprintf("close-%s-%s-%d",
		position.ID,
		position.Expiration.Format("20060102"),
		time.Now().Unix())

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

		closeOrder, err := c.broker.CloseStranglePositionCtx(
			closeCtx,
			position.Symbol,
			position.PutStrike,
			position.CallStrike,
			position.Expiration.Format("2006-01-02"),
			position.Quantity,
			maxDebit,
			clientOrderID,
		)

		if err == nil {
			// OrderResponse may contain a zero-value Order; avoid nil comparisons on structs.
			c.logger.Printf("Close order placed successfully on attempt %d", attempt+1)
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
			c.logger.Printf("Failed to generate jitter: %v", err)
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
		"i/o timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"temporarily unavailable",
		"server error",
		"rate limit",
		"429", // HTTP 429 Too Many Requests
		"502", // HTTP 502 Bad Gateway
		"503", // HTTP 503 Service Unavailable
		"504", // HTTP 504 Gateway Timeout
		"network",
		"dns",
		"tcp",
		"no such host",
		"deadline exceeded",
		"tls handshake",
		"broken pipe",
		"eof",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}
