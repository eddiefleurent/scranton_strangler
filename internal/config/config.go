// Package config provides configuration management for the trading bot.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Risk Management Constants
const (
	// EscalateLossPct represents the 200% loss threshold for escalating/preparing for action
	EscalateLossPct = 2.0
	// StopLossPct represents the 250% loss threshold for immediate position closure
	StopLossPct = 2.5
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
}

// StrategyConfig defines trading strategy parameters.
type StrategyConfig struct {
	Symbol              string           `yaml:"symbol"`
	Entry               EntryConfig      `yaml:"entry"`
	Exit                ExitConfig       `yaml:"exit"`
	Adjustments         AdjustmentConfig `yaml:"adjustments"`
	AllocationPct       float64          `yaml:"allocation_pct"`
	UseMockHistoricalIV bool             `yaml:"use_mock_historical_iv"`
}

type EntryConfig struct {
	DTERange  []int   `yaml:"dte_range"`
	MinIVR    float64 `yaml:"min_ivr"`
	TargetDTE int     `yaml:"target_dte"`
	Delta     float64 `yaml:"delta"`
	MinCredit float64 `yaml:"min_credit"`
}

type ExitConfig struct {
	ProfitTarget float64 `yaml:"profit_target"`
	MaxDTE       int     `yaml:"max_dte"`
}

type AdjustmentConfig struct {
	Enabled             bool    `yaml:"enabled"`
	SecondDownThreshold float64 `yaml:"second_down_threshold"`
}

type RiskConfig struct {
	MaxContracts    int     `yaml:"max_contracts"`
	MaxDailyLoss    float64 `yaml:"max_daily_loss"`
	MaxPositionLoss float64 `yaml:"max_position_loss"`
}

type ScheduleConfig struct {
	MarketCheckInterval string `yaml:"market_check_interval"`
	TradingStart        string `yaml:"trading_start"`
	TradingEnd          string `yaml:"trading_end"`
	AfterHoursCheck     bool   `yaml:"after_hours_check"`
}

func Load(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = "config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Validate config
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

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
	if c.Strategy.AllocationPct <= 0 || c.Strategy.AllocationPct > 0.5 {
		return fmt.Errorf("strategy.allocation_pct must be between 0 and 0.5")
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
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return fmt.Errorf("failed to load timezone: %w", err)
	}
	s, err1 := time.ParseInLocation("15:04", c.Schedule.TradingStart, loc)
	e, err2 := time.ParseInLocation("15:04", c.Schedule.TradingEnd, loc)
	if err1 != nil || err2 != nil || (s.Hour() > e.Hour() || (s.Hour() == e.Hour() && s.Minute() >= e.Minute())) {
		return fmt.Errorf("schedule trading window invalid (start/end parse/order)")
	}

	return nil
}

func (c *Config) IsPaperTrading() bool {
	return c.Environment.Mode == "paper"
}

func (c *Config) GetCheckInterval() time.Duration {
	d, err := time.ParseDuration(c.Schedule.MarketCheckInterval)
	if err != nil {
		return 15 * time.Minute // default
	}
	return d
}

func (c *Config) IsWithinTradingHours(now time.Time) bool {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.FixedZone("ET", -5*60*60) // fallback, DST-agnostic
	}
	todayET := now.In(loc)

	// Only allow Monday–Friday trading
	if todayET.Weekday() == time.Saturday || todayET.Weekday() == time.Sunday {
		return false
	}

	startClock, err1 := time.ParseInLocation("15:04", c.Schedule.TradingStart, loc)
	endClock, err2 := time.ParseInLocation("15:04", c.Schedule.TradingEnd, loc)
	if err1 != nil || err2 != nil {
		// Safe defaults if misconfigured
		startClock, _ = time.ParseInLocation("15:04", "09:45", loc) //nolint:errcheck // hardcoded default
		endClock, _ = time.ParseInLocation("15:04", "15:45", loc)   //nolint:errcheck // hardcoded default
	}
	start := time.Date(todayET.Year(), todayET.Month(), todayET.Day(),
		startClock.Hour(), startClock.Minute(), 0, 0, loc)
	end := time.Date(todayET.Year(), todayET.Month(), todayET.Day(),
		endClock.Hour(), endClock.Minute(), 0, 0, loc)

	// Inclusive start, exclusive end
	return !todayET.Before(start) && todayET.Before(end)
}

// normalizeExitConfig sets default values for exit configuration
func (c *Config) normalizeExitConfig() {
	if c.Strategy.Exit.MaxDTE == 0 {
		c.Strategy.Exit.MaxDTE = defaultMaxDTE
	}
}

// GetMaxDTE returns the configured MaxDTE value, falling back to defaultMaxDTE if unset
func (c *Config) GetMaxDTE() int {
	if c.Strategy.Exit.MaxDTE == 0 {
		return defaultMaxDTE
	}
	return c.Strategy.Exit.MaxDTE
}
