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
	// Ratio of position credit (e.g., 2.0 = 200% loss triggers escalation)
	defaultEscalateLossPct = 2.0
	// defaultStopLossPct is used when strategy.exit.stop_loss_pct is unset
	// Ratio of position credit (e.g., 2.5 = 250% loss triggers hard stop)
	defaultStopLossPct = 2.5
	// defaultRiskMaxPositionLoss is used when risk.max_position_loss is unset
	// Percent of account equity (e.g., 3.0 = 3% of account value)
	defaultRiskMaxPositionLoss = 3.0
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
	Dashboard   DashboardConfig   `yaml:"dashboard"`
}

// EnvironmentConfig defines the environment settings.
type EnvironmentConfig struct {
	Mode     string `yaml:"mode"`      // paper | live
	LogLevel string `yaml:"log_level"` // debug | info | warn | error
}

// BrokerConfig defines broker API settings.
type BrokerConfig struct {
	Provider         string        `yaml:"provider"`
	APIKey           string        `yaml:"api_key"`
	AccountID        string        `yaml:"account_id"`
	UseOTOCO         bool          `yaml:"use_otoco"` // Use OTOCO orders for preset exits
	// OTOCOPreview enables preview validation for OTOCO orders before placement
	OTOCOPreview     bool          `yaml:"otoco_preview"`
	// OTOCOFallback enables fallback to separate orders if OTOCO validation fails
	OTOCOFallback    bool          `yaml:"otoco_fallback"`
	// PhantomThreshold defines how long to wait before cleaning up phantom positions (quantity=0, credit=0)
	PhantomThreshold time.Duration `yaml:"phantom_threshold"`
}

// StrategyConfig defines trading strategy parameters.
type StrategyConfig struct {
	Symbol                  string           `yaml:"symbol"`
	Entry                   EntryConfig      `yaml:"entry"`
	Exit                    ExitConfig       `yaml:"exit"`
	Adjustments             AdjustmentConfig `yaml:"adjustments"`
	AllocationPct           float64          `yaml:"allocation_pct"`
	EscalateLossPct         float64          `yaml:"escalate_loss_pct"`
	MaxNewPositionsPerCycle int              `yaml:"max_new_positions_per_cycle"`
}

// EntryConfig defines entry criteria for opening new positions.
type EntryConfig struct {
	MinIVPct        float64 `yaml:"min_iv_pct"`         // Minimum SPY ATM IV percentage to enter
	DTERange        []int   `yaml:"dte_range"`
	TargetDTE       int     `yaml:"target_dte"`
	Delta           float64 `yaml:"delta"` // Delta in percentage points (e.g., 16 = 0.16 fractional)
	MinCredit       float64 `yaml:"min_credit"`
	MinVolume       int64   `yaml:"min_volume"`         // Minimum daily volume for liquidity filtering
	MinOpenInterest int64   `yaml:"min_open_interest"`  // Minimum open interest for liquidity filtering
}

// ExitConfig defines exit criteria for closing positions.
type ExitConfig struct {
	ProfitTarget float64 `yaml:"profit_target"` // Fraction (e.g., 0.25 = 25%)
	MaxDTE       int     `yaml:"max_dte"`
	StopLossPct  float64 `yaml:"stop_loss_pct"`
}

// AdjustmentConfig defines parameters for position adjustments.
type AdjustmentConfig struct {
	Enabled             bool    `yaml:"enabled"`
	SecondDownThreshold float64 `yaml:"second_down_threshold"` // Percent of position credit (e.g., 1.5 = 1.5% of credit received)
	EnableAdjustmentStub bool   `yaml:"enable_adjustment_stub"` // Feature flag to enable adjustment stub logging
}

// RiskConfig defines risk management parameters.
type RiskConfig struct {
	MaxContracts    int     `yaml:"max_contracts"`     // Maximum number of contracts per position
	MaxPositions    int     `yaml:"max_positions"`     // Maximum number of concurrent positions
	MaxDailyLoss    float64 `yaml:"max_daily_loss"`    // Percent of account equity (e.g., 5.0 = 5% of account value)
	MaxPositionLoss float64 `yaml:"max_position_loss"` // Percent of account equity (e.g., 3.0 = 3% of account value)
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

// DashboardConfig defines web dashboard settings.
type DashboardConfig struct {
	Enabled   bool   `yaml:"enabled"`    // Enable web dashboard
	Port      int    `yaml:"port"`       // HTTP server port
	AuthToken string `yaml:"auth_token"` // Optional authentication token
}

// Load reads and parses the configuration file from the specified path.
func Load(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = "config.yaml"
	}

	data, err := os.ReadFile(configPath) // #nosec G304 -- configPath is a user-provided config file path
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", configPath, err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var config Config
	dec := yaml.NewDecoder(strings.NewReader(expanded))
	dec.KnownFields(true)
	if err := dec.Decode(&config); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", configPath, err)
	}

	// Normalize config defaults
	config.Normalize()

	// Validate config
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// resolveLocation returns the configured TZ or NY fallback.
// With embedded tzdata, LoadLocation should always succeed for valid timezones.
func (c *Config) resolveLocation() (*time.Location, error) {
	tz := c.Schedule.Timezone
	if strings.TrimSpace(tz) == "" {
		tz = "America/New_York"
	}
	
	loc, err := time.LoadLocation(tz)
	if err != nil {
		// With embedded tzdata, this should only fail for invalid timezone names
		return nil, fmt.Errorf("failed to load timezone %q: %w", tz, err)
	}
	
	return loc, nil
}

// Validate checks that all configuration values are valid and consistent.
func (c *Config) Validate() error {
	// Environment validation
	if c.Environment.Mode != "paper" && c.Environment.Mode != "live" {
		return fmt.Errorf("environment.mode must be 'paper' or 'live'")
	}

	// Log level validation
	switch strings.ToLower(c.Environment.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("environment.log_level must be one of: debug, info, warn, error")
	}

	// OTOCO validation - reject OTOCO flags in live mode as they are unimplemented no-ops
	if c.Environment.Mode == "live" && (c.Broker.UseOTOCO || c.Broker.OTOCOPreview || c.Broker.OTOCOFallback) {
		return fmt.Errorf("OTOCO flags are not allowed in live mode; these features are unimplemented no-ops")
	}

	// Broker validation
	if strings.TrimSpace(c.Broker.APIKey) == "" {
		return fmt.Errorf("broker.api_key is required")
	}
	if strings.TrimSpace(c.Broker.AccountID) == "" {
		return fmt.Errorf("broker.account_id is required")
	}

	// Provider validation
	switch strings.ToLower(c.Broker.Provider) {
	case "tradier":
	default:
		return fmt.Errorf("broker.provider must be 'tradier'")
	}

	// Phantom threshold validation
	if c.Broker.PhantomThreshold < 0 {
		return fmt.Errorf("broker.phantom_threshold must be >= 0")
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
	if c.Strategy.MaxNewPositionsPerCycle <= 0 {
		return fmt.Errorf("strategy.max_new_positions_per_cycle must be > 0")
	}
	if c.Strategy.Entry.MinIVPct <= 0 || c.Strategy.Entry.MinIVPct > 100 {
		return fmt.Errorf("strategy.entry.min_iv_pct must be between 0 and 100")
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
	if c.Strategy.Entry.MinVolume < 0 {
		return fmt.Errorf("strategy.entry.min_volume must be >= 0")
	}
	if c.Strategy.Entry.MinOpenInterest < 0 {
		return fmt.Errorf("strategy.entry.min_open_interest must be >= 0")
	}

	// Exit configuration validation
	if c.Strategy.Exit.ProfitTarget <= 0 || c.Strategy.Exit.ProfitTarget >= 1 {
		return fmt.Errorf("strategy.exit.profit_target must be in (0,1)")
	}
	if c.Strategy.Exit.StopLossPct <= 0 {
		return fmt.Errorf("strategy.exit.stop_loss_pct must be > 0")
	}
	if c.Strategy.Exit.MaxDTE <= 0 {
		return fmt.Errorf("strategy.exit.max_dte must be > 0")
	}

	// Adjustment thresholds must be sensible relative to stop loss
	if c.Strategy.Adjustments.Enabled {
		if c.Strategy.Adjustments.SecondDownThreshold <= 0 {
			return fmt.Errorf("strategy.adjustments.second_down_threshold must be > 0 when adjustments.enabled is true")
		}
		if c.Strategy.Adjustments.SecondDownThreshold >= c.Strategy.Exit.StopLossPct {
			return fmt.Errorf("strategy.adjustments.second_down_threshold (%.2f) must be < strategy.exit.stop_loss_pct (%.2f)",
				c.Strategy.Adjustments.SecondDownThreshold, c.Strategy.Exit.StopLossPct)
		}
	}

	// Validate loss percentage constraints
	if c.Strategy.EscalateLossPct >= c.Strategy.Exit.StopLossPct {
		return fmt.Errorf("strategy.escalate_loss_pct (%.2f) must be < strategy.exit.stop_loss_pct (%.2f)",
			c.Strategy.EscalateLossPct, c.Strategy.Exit.StopLossPct)
	}
	// Note: Cross-unit comparison between StopLossPct (position credit %) and
	// MaxPositionLoss (account equity %) is invalid and removed. Runtime logic
	// in strategy handles clamping stop losses to risk caps when position context is available.

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
	if c.Schedule.MarketCheckInterval == "" {
		return fmt.Errorf("schedule.market_check_interval is required (set in Normalize)")
	}
	trimmedInterval := strings.TrimSpace(c.Schedule.MarketCheckInterval)
	if duration, err := time.ParseDuration(trimmedInterval); err != nil {
		return fmt.Errorf("schedule.market_check_interval invalid: %w", err)
	} else if duration <= 0 {
		return fmt.Errorf("schedule.market_check_interval must be > 0")
	}
	loc, err := c.resolveLocation()
	if err != nil {
		return fmt.Errorf("timezone resolution failed: %w", err)
	}
	s, err1 := time.ParseInLocation("15:04", c.Schedule.TradingStart, loc)
	e, err2 := time.ParseInLocation("15:04", c.Schedule.TradingEnd, loc)
	if err1 != nil || err2 != nil || !s.Before(e) {
		return fmt.Errorf("schedule trading window invalid (start/end parse/order)")
	}

	// Storage validation
	if strings.TrimSpace(c.Storage.Path) == "" {
		return fmt.Errorf("storage.path is required")
	}

	// Dashboard validation
	if c.Dashboard.Enabled {
		if c.Dashboard.Port <= 0 || c.Dashboard.Port > 65535 {
			return fmt.Errorf("dashboard.port must be between 1 and 65535")
		}
	}

	return nil
}

// IsPaperTrading returns true if the bot is configured for paper trading.
func (c *Config) IsPaperTrading() bool {
	return c.Environment.Mode == "paper"
}

// GetCheckInterval returns the configured market check interval duration.
func (c *Config) GetCheckInterval() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(c.Schedule.MarketCheckInterval))
	if err != nil {
		return 15 * time.Minute // default
	}
	if d <= 0 {
		return 15 * time.Minute // default
	}
	return d
}

// IsWithinTradingHours checks if the given time falls within configured trading hours.
func (c *Config) IsWithinTradingHours(now time.Time) (bool, error) {
	loc, err := c.resolveLocation()
	if err != nil {
		return false, fmt.Errorf("timezone resolution failed: %w", err)
	}
	
	today := now.In(loc)

	// Only allow Monday–Friday trading
	if today.Weekday() == time.Saturday || today.Weekday() == time.Sunday {
		return false, nil
	}

	// Allow early return for AfterHoursCheck only on weekdays
	if c.Schedule.AfterHoursCheck {
		return true, nil
	}

	startClock, err1 := time.ParseInLocation("15:04", c.Schedule.TradingStart, loc)
	endClock, err2 := time.ParseInLocation("15:04", c.Schedule.TradingEnd, loc)
	if err1 != nil || err2 != nil {
		// Safe defaults if misconfigured
		startClock = time.Date(0, 1, 1, 9, 30, 0, 0, loc)
		endClock = time.Date(0, 1, 1, 16, 0, 0, 0, loc)
	}
	start := time.Date(today.Year(), today.Month(), today.Day(),
		startClock.Hour(), startClock.Minute(), 0, 0, loc)
	end := time.Date(today.Year(), today.Month(), today.Day(),
		endClock.Hour(), endClock.Minute(), 0, 0, loc)

	// Inclusive start, exclusive end
	return !today.Before(start) && today.Before(end), nil
}

// Normalize sets default values for configuration fields
func (c *Config) Normalize() {
	if strings.TrimSpace(c.Schedule.MarketCheckInterval) == "" {
		c.Schedule.MarketCheckInterval = "15m"
	}
	if strings.TrimSpace(c.Environment.Mode) == "" {
		c.Environment.Mode = "paper"
	}
	if strings.TrimSpace(c.Environment.LogLevel) == "" {
		c.Environment.LogLevel = "info"
	}
	if c.Strategy.Exit.MaxDTE == 0 {
		c.Strategy.Exit.MaxDTE = defaultMaxDTE
	}
	if c.Strategy.EscalateLossPct == 0 {
		c.Strategy.EscalateLossPct = defaultEscalateLossPct
	}
	if c.Risk.MaxPositionLoss == 0 {
		c.Risk.MaxPositionLoss = defaultRiskMaxPositionLoss
	}
	if c.Strategy.Exit.StopLossPct == 0 {
		// StopLossPct uses credit units and is not constrained by MaxPositionLoss (equity units)
		c.Strategy.Exit.StopLossPct = defaultStopLossPct
	}
	if c.Strategy.MaxNewPositionsPerCycle == 0 {
		c.Strategy.MaxNewPositionsPerCycle = 1 // Default to 1 for safety
	}
	if c.Dashboard.Port == 0 {
		c.Dashboard.Port = 9847 // Default port as specified in tasks
	}
	if c.Broker.PhantomThreshold == 0 {
		c.Broker.PhantomThreshold = 10 * time.Minute // Default to 10 minutes
	}
}

// GetMaxDTE returns the configured MaxDTE value, falling back to defaultMaxDTE if unset
func (c *Config) GetMaxDTE() int {
	if c.Strategy.Exit.MaxDTE == 0 {
		return defaultMaxDTE
	}
	return c.Strategy.Exit.MaxDTE
}
