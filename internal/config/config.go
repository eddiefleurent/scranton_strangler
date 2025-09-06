// Package config provides configuration management for the trading bot.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v3"
)

// Risk Management Constants
const (
	// defaultEscalateLossPct is used when strategy.escalate_loss_pct is unset
	defaultEscalateLossPct = 2.0
	// defaultStopLossPct is used when strategy.exit.stop_loss_pct is unset
	defaultStopLossPct = 2.5
	// defaultMaxDTE represents the default maximum days to expiration before forced exit (21 days)
	defaultMaxDTE = 21
)

// Config represents the complete application configuration.
type Config struct {
	Environment EnvironmentConfig `yaml:"environment"`
	Broker      BrokerConfig      `yaml:"broker"`
	Schedule    ScheduleConfig    `yaml:"schedule"`
	Strategy    StrategyConfig    `yaml:"strategy"`
	Risk        RiskConfig        `yaml:"risk"`
	Storage     StorageConfig     `yaml:"storage"`
}

// EnvironmentConfig defines the environment settings.
type EnvironmentConfig struct {
	Mode     string `yaml:"mode"`      // paper | live
	LogLevel string `yaml:"log_level"` // debug | info | warn | error
}

// BrokerConfig defines broker API settings.
type BrokerConfig struct {
	Provider    string `yaml:"provider"`
	APIKey      string `yaml:"api_key"`
	APIEndpoint string `yaml:"api_endpoint"`
	AccountID   string `yaml:"account_id"`
	UseOTOCO    bool   `yaml:"use_otoco"` // Use OTOCO orders for preset exits
	// OTOCOPreview enables preview validation for OTOCO orders before placement
	OTOCOPreview bool `yaml:"otoco_preview"`
	// OTOCOFallback enables fallback to separate orders if OTOCO validation fails
	OTOCOFallback bool `yaml:"otoco_fallback"`
}

// StrategyConfig defines trading strategy parameters.
type StrategyConfig struct {
	Symbol              string           `yaml:"symbol"`
	Entry               EntryConfig      `yaml:"entry"`
	Exit                ExitConfig       `yaml:"exit"`
	Adjustments         AdjustmentConfig `yaml:"adjustments"`
	AllocationPct       float64          `yaml:"allocation_pct"`
	EscalateLossPct     float64          `yaml:"escalate_loss_pct"`
	UseMockHistoricalIV bool             `yaml:"use_mock_historical_iv"`
}

// EntryConfig defines entry criteria for opening new positions.
type EntryConfig struct {
	DTERange  []int   `yaml:"dte_range"`
	MinIVR    float64 `yaml:"min_ivr"`
	TargetDTE int     `yaml:"target_dte"`
	Delta     float64 `yaml:"delta"`
	MinCredit float64 `yaml:"min_credit"`
}

// ExitConfig defines exit criteria for closing positions.
type ExitConfig struct {
	ProfitTarget float64 `yaml:"profit_target"`
	MaxDTE       int     `yaml:"max_dte"`
	StopLossPct  float64 `yaml:"stop_loss_pct"`
}

// AdjustmentConfig defines parameters for position adjustments.
type AdjustmentConfig struct {
	Enabled             bool    `yaml:"enabled"`
	SecondDownThreshold float64 `yaml:"second_down_threshold"`
}

// RiskConfig defines risk management parameters.
type RiskConfig struct {
	MaxContracts    int     `yaml:"max_contracts"`
	MaxDailyLoss    float64 `yaml:"max_daily_loss"`
	MaxPositionLoss float64 `yaml:"max_position_loss"`
}

// ScheduleConfig defines trading schedule and market hours.
type ScheduleConfig struct {
	MarketCheckInterval string `yaml:"market_check_interval"`
	Timezone            string `yaml:"timezone"`      // e.g., "America/New_York"
	TradingStart        string `yaml:"trading_start"` // "HH:MM"
	TradingEnd          string `yaml:"trading_end"`   // "HH:MM"
	AfterHoursCheck     bool   `yaml:"after_hours_check"`
}

// StorageConfig defines storage settings for position data.
type StorageConfig struct {
	Path string `yaml:"path"`
}

// Load reads and parses the configuration file from the specified path.
func Load(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = "config.yaml"
	}

	data, err := os.ReadFile(configPath) // #nosec G304 -- configPath is a user-provided config file path
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var config Config
	dec := yaml.NewDecoder(strings.NewReader(expanded))
	dec.KnownFields(true)
	if err := dec.Decode(&config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Validate config
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// Validate checks that all configuration values are valid and consistent.
func (c *Config) Validate() error {
	// Environment validation
	if c.Environment.Mode != "paper" && c.Environment.Mode != "live" {
		return fmt.Errorf("environment.mode must be 'paper' or 'live'")
	}

	// Broker validation
	if c.Broker.APIKey == "" {
		return fmt.Errorf("broker.api_key is required")
	}
	if c.Broker.AccountID == "" {
		return fmt.Errorf("broker.account_id is required")
	}

	// Strategy validation
	if c.Strategy.Symbol == "" {
		return fmt.Errorf("strategy.symbol is required")
	}
	if c.Strategy.AllocationPct <= 0 || c.Strategy.AllocationPct > 1.0 {
		return fmt.Errorf("strategy.allocation_pct must be between 0 and 1.0")
	}
	if c.Strategy.EscalateLossPct <= 0 {
		return fmt.Errorf("strategy.escalate_loss_pct must be > 0")
	}
	if c.Strategy.Entry.MinIVR < 0 || c.Strategy.Entry.MinIVR > 100 {
		return fmt.Errorf("strategy.entry.min_ivr must be between 0 and 100")
	}
	if c.Strategy.Entry.Delta <= 0 || c.Strategy.Entry.Delta > 50 {
		return fmt.Errorf("strategy.entry.delta must be between 0 and 50")
	}
	// DTE range must be [min,max] with positive ints and min <= max
	if len(c.Strategy.Entry.DTERange) != 2 ||
		c.Strategy.Entry.DTERange[0] <= 0 ||
		c.Strategy.Entry.DTERange[1] <= 0 ||
		c.Strategy.Entry.DTERange[0] > c.Strategy.Entry.DTERange[1] {
		return fmt.Errorf("strategy.entry.dte_range must be [min,max] with positive values and min <= max")
	}
	if c.Strategy.Entry.TargetDTE <= 0 {
		return fmt.Errorf("strategy.entry.target_dte must be > 0")
	}
	{
		minDTE, maxDTE := c.Strategy.Entry.DTERange[0], c.Strategy.Entry.DTERange[1]
		if c.Strategy.Entry.TargetDTE < minDTE || c.Strategy.Entry.TargetDTE > maxDTE {
			return fmt.Errorf("strategy.entry.target_dte (%d) must be within dte_range [%d,%d]",
				c.Strategy.Entry.TargetDTE, minDTE, maxDTE)
		}
	}
	if c.Strategy.Entry.MinCredit <= 0 {
		return fmt.Errorf("strategy.entry.min_credit must be > 0")
	}

	// Normalize exit configuration
	c.normalizeExitConfig()

	// Exit configuration validation
	if c.Strategy.Exit.ProfitTarget <= 0 || c.Strategy.Exit.ProfitTarget >= 1 {
		return fmt.Errorf("strategy.exit.profit_target must be in (0,1)")
	}
	if c.Strategy.Exit.StopLossPct <= 0 {
		return fmt.Errorf("strategy.exit.stop_loss_pct must be > 0")
	}

	// Validate loss percentage constraints
	if c.Strategy.EscalateLossPct >= c.Strategy.Exit.StopLossPct {
		return fmt.Errorf("strategy.escalate_loss_pct (%.2f) must be < strategy.exit.stop_loss_pct (%.2f)",
			c.Strategy.EscalateLossPct, c.Strategy.Exit.StopLossPct)
	}
	// Note: strategy.exit.stop_loss_pct takes precedence over risk.max_position_loss when both are configured
	// This allows for different loss thresholds at strategy vs risk management levels
	if c.Strategy.Exit.StopLossPct > c.Risk.MaxPositionLoss {
		return fmt.Errorf("strategy.exit.stop_loss_pct (%.2f) must be <= risk.max_position_loss (%.2f)",
			c.Strategy.Exit.StopLossPct, c.Risk.MaxPositionLoss)
	}

	// Risk validation
	if c.Risk.MaxContracts <= 0 {
		return fmt.Errorf("risk.max_contracts must be > 0")
	}
	if c.Risk.MaxDailyLoss <= 0 {
		return fmt.Errorf("risk.max_daily_loss must be > 0")
	}
	if c.Risk.MaxPositionLoss <= 0 {
		return fmt.Errorf("risk.max_position_loss must be > 0")
	}

	// Schedule validation
	if _, err := time.ParseDuration(c.Schedule.MarketCheckInterval); err != nil {
		return fmt.Errorf("schedule.market_check_interval invalid: %w", err)
	}
	tz := c.Schedule.Timezone
	if tz == "" {
		tz = "America/New_York"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		// Fallback for minimal containers
		loc = time.FixedZone("ET", -5*60*60)
	}
	s, err1 := time.ParseInLocation("15:04", c.Schedule.TradingStart, loc)
	e, err2 := time.ParseInLocation("15:04", c.Schedule.TradingEnd, loc)
	if err1 != nil || err2 != nil || (s.Hour() > e.Hour() || (s.Hour() == e.Hour() && s.Minute() >= e.Minute())) {
		return fmt.Errorf("schedule trading window invalid (start/end parse/order)")
	}

	return nil
}

// IsPaperTrading returns true if the bot is configured for paper trading.
func (c *Config) IsPaperTrading() bool {
	return c.Environment.Mode == "paper"
}

// GetCheckInterval returns the configured market check interval duration.
func (c *Config) GetCheckInterval() time.Duration {
	d, err := time.ParseDuration(c.Schedule.MarketCheckInterval)
	if err != nil {
		return 15 * time.Minute // default
	}
	return d
}

// IsWithinTradingHours checks if the given time falls within configured trading hours.
func (c *Config) IsWithinTradingHours(now time.Time) bool {
	tz := c.Schedule.Timezone
	if tz == "" {
		tz = "America/New_York"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		// Try fallback to America/New_York
		if fallbackLoc, err2 := time.LoadLocation("America/New_York"); err2 == nil {
			loc = fallbackLoc
		} else {
			// Final fallback to DST-agnostic FixedZone
			loc = time.FixedZone("ET", -5*60*60)
		}
	}
	today := now.In(loc)

	// Only allow Monday–Friday trading
	if today.Weekday() == time.Saturday || today.Weekday() == time.Sunday {
		return false
	}

	startClock, err1 := time.ParseInLocation("15:04", c.Schedule.TradingStart, loc)
	endClock, err2 := time.ParseInLocation("15:04", c.Schedule.TradingEnd, loc)
	if err1 != nil || err2 != nil {
		// Safe defaults if misconfigured
		startClock = time.Date(0, 1, 1, 9, 45, 0, 0, loc)
		endClock = time.Date(0, 1, 1, 15, 45, 0, 0, loc)
	}
	start := time.Date(today.Year(), today.Month(), today.Day(),
		startClock.Hour(), startClock.Minute(), 0, 0, loc)
	end := time.Date(today.Year(), today.Month(), today.Day(),
		endClock.Hour(), endClock.Minute(), 0, 0, loc)

	// Inclusive start, exclusive end
	return !today.Before(start) && today.Before(end)
}

// normalizeExitConfig sets default values for exit configuration
func (c *Config) normalizeExitConfig() {
	if c.Strategy.Exit.MaxDTE == 0 {
		c.Strategy.Exit.MaxDTE = defaultMaxDTE
	}
	if c.Strategy.EscalateLossPct == 0 {
		c.Strategy.EscalateLossPct = defaultEscalateLossPct
	}
	if c.Strategy.Exit.StopLossPct == 0 {
		c.Strategy.Exit.StopLossPct = defaultStopLossPct
	}
}

// GetMaxDTE returns the configured MaxDTE value, falling back to defaultMaxDTE if unset
func (c *Config) GetMaxDTE() int {
	if c.Strategy.Exit.MaxDTE == 0 {
		return defaultMaxDTE
	}
	return c.Strategy.Exit.MaxDTE
}
