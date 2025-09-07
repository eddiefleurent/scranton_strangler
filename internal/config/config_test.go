package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Test with example config file (should work for basic structure validation)
	configPath := filepath.Join("..", "..", "config.yaml.example")
	_, err := Load(configPath)
	if err != nil {
		t.Errorf("Expected config to load successfully from example file, got error: %v", err)
	}
}

func TestLoad_InvalidPath(t *testing.T) {
	_, err := Load("nonexistent.yaml")
	if err == nil {
		t.Error("Expected error when loading nonexistent config file, got nil")
	}
}

func TestValidate_LossPercentageConstraints(t *testing.T) {
	// Create a base valid config
	baseConfig := &Config{
		Environment: EnvironmentConfig{
			Mode:     "paper",
			LogLevel: "info",
		},
		Broker: BrokerConfig{
			Provider:    "tradier",
			APIKey:      "test-key",
			APIEndpoint: "https://sandbox.tradier.com/v1",
			AccountID:   "test-account",
			UseOTOCO:    true,
		},
		Strategy: StrategyConfig{
			Symbol:              "SPY",
			AllocationPct:       0.35,
			EscalateLossPct:     2.0,
			UseMockHistoricalIV: false,
			Entry: EntryConfig{
				MinIVR:    30,
				TargetDTE: 45,
				DTERange:  []int{40, 50},
				Delta:     16,
				MinCredit: 2.00,
			},
			Exit: ExitConfig{
				ProfitTarget: 0.50,
				MaxDTE:       21,
				StopLossPct:  2.5,
			},
			Adjustments: AdjustmentConfig{
				Enabled:             false,
				SecondDownThreshold: 10,
			},
		},
		Risk: RiskConfig{
			MaxContracts:    1,
			MaxDailyLoss:    500,
			MaxPositionLoss: 2.0,
		},
		Schedule: ScheduleConfig{
			MarketCheckInterval: "15m",
			TradingStart:        "09:45",
			TradingEnd:          "15:45",
			AfterHoursCheck:     false,
		},
		Storage: StorageConfig{
			Path: "positions.json",
		},
	}

	t.Run("valid loss percentages", func(t *testing.T) {
		config := *baseConfig
		config.Strategy.EscalateLossPct = 1.5
		config.Strategy.Exit.StopLossPct = 2.0
		config.Risk.MaxPositionLoss = 2.5

		err := config.Validate()
		if err != nil {
			t.Errorf("Expected valid config, got error: %v", err)
		}
	})

	t.Run("escalate_loss_pct equal to stop_loss_pct - invalid", func(t *testing.T) {
		config := *baseConfig
		config.Strategy.EscalateLossPct = 2.5
		config.Strategy.Exit.StopLossPct = 2.5

		err := config.Validate()
		if err == nil {
			t.Error("Expected error when escalate_loss_pct equals stop_loss_pct")
		}
		expectedMsg := "strategy.escalate_loss_pct (2.50) must be < strategy.exit.stop_loss_pct (2.50)"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %v", expectedMsg, err)
		}
	})

	t.Run("escalate_loss_pct greater than stop_loss_pct - invalid", func(t *testing.T) {
		config := *baseConfig
		config.Strategy.EscalateLossPct = 3.0
		config.Strategy.Exit.StopLossPct = 2.5

		err := config.Validate()
		if err == nil {
			t.Error("Expected error when escalate_loss_pct > stop_loss_pct")
		}
		expectedMsg := "strategy.escalate_loss_pct (3.00) must be < strategy.exit.stop_loss_pct (2.50)"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %v", expectedMsg, err)
		}
	})

	t.Run("stop_loss_pct greater than max_position_loss - invalid", func(t *testing.T) {
		config := *baseConfig
		config.Strategy.EscalateLossPct = 1.5
		config.Strategy.Exit.StopLossPct = 2.5
		config.Risk.MaxPositionLoss = 2.0

		err := config.Validate()
		if err == nil {
			t.Error("Expected error when stop_loss_pct > max_position_loss")
		}
		expectedMsg := "strategy.exit.stop_loss_pct (2.50) must be <= risk.max_position_loss (2.00)"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %v", expectedMsg, err)
		}
	})

	t.Run("stop_loss_pct equal to max_position_loss - valid", func(t *testing.T) {
		config := *baseConfig
		config.Strategy.EscalateLossPct = 1.5
		config.Strategy.Exit.StopLossPct = 2.0
		config.Risk.MaxPositionLoss = 2.0

		err := config.Validate()
		if err != nil {
			t.Errorf("Expected valid config when stop_loss_pct equals max_position_loss, got error: %v", err)
		}
	})

	t.Run("escalate_loss_pct zero - invalid", func(t *testing.T) {
		config := *baseConfig
		config.Strategy.EscalateLossPct = 0

		err := config.Validate()
		if err == nil {
			t.Error("Expected error when escalate_loss_pct is 0")
		}
		expectedMsg := "strategy.escalate_loss_pct must be > 0"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %v", expectedMsg, err)
		}
	})

	t.Run("escalate_loss_pct negative - invalid", func(t *testing.T) {
		config := *baseConfig
		config.Strategy.EscalateLossPct = -1.0

		err := config.Validate()
		if err == nil {
			t.Error("Expected error when escalate_loss_pct is negative")
		}
		expectedMsg := "strategy.escalate_loss_pct must be > 0"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %v", expectedMsg, err)
		}
	})

	t.Run("boundary values - valid ascending order", func(t *testing.T) {
		config := *baseConfig
		config.Strategy.EscalateLossPct = 1.0
		config.Strategy.Exit.StopLossPct = 1.5
		config.Risk.MaxPositionLoss = 2.0

		err := config.Validate()
		if err != nil {
			t.Errorf("Expected valid config with proper ascending order, got error: %v", err)
		}
	})
}

func TestLoad_UnknownFields(t *testing.T) {
	const badYAML = `
environment: { mode: "paper", log_level: "info" }
broker: { provider: "tradier", api_key: "k", api_endpoint: "x", account_id: "a" }
strategy:
  symbol: "SPY"
  allocation_pct: 0.3
  escalate_loss_pct: 2.0
  entry: { min_ivr: 30, target_dte: 45, dte_range: [40,50], delta: 16, min_credit: 2.0 }
  exit: { profit_target: 0.5, max_dte: 21, stop_loss_pct: 2.5 }
risk: { max_contracts: 1, max_daily_loss: 500, max_position_loss: 2.0 }
schedule: { market_check_interval: "15m", trading_start: "09:45", trading_end: "15:45", after_hours_check: false }
storage: { path: "positions.json" }
extra_unknown_key: true
`
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.yaml")
	if err := os.WriteFile(path, []byte(badYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestConfig_IsWithinTradingHours(t *testing.T) {
	tests := []struct {
		name     string
		timeStr  string
		expected bool
	}{
		{
			name:     "during trading hours",
			timeStr:  "2024-01-08T10:00:00-05:00", // Monday 10:00 AM ET
			expected: true,
		},
		{
			name:     "before trading hours",
			timeStr:  "2024-01-08T09:00:00-05:00", // Monday 9:00 AM ET
			expected: false,
		},
		{
			name:     "after trading hours",
			timeStr:  "2024-01-08T16:00:00-05:00", // Monday 4:00 PM ET
			expected: false,
		},
		{
			name:     "weekend",
			timeStr:  "2024-01-06T10:00:00-05:00", // Saturday 10:00 AM ET
			expected: false,
		},
	}

	config := &Config{
		Schedule: ScheduleConfig{
			TradingStart: "09:45",
			TradingEnd:   "15:45",
			Timezone:     "America/New_York",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testTime, err := time.Parse(time.RFC3339, tt.timeStr)
			if err != nil {
				t.Fatalf("failed to parse test time: %v", err)
			}

			result := config.IsWithinTradingHours(testTime)
			if result != tt.expected {
				t.Errorf("IsWithinTradingHours() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConfig_AfterHoursCheck(t *testing.T) {
	tests := []struct {
		name            string
		afterHoursCheck bool
		timeStr         string
		expectSkip      bool
	}{
		{
			name:            "regular hours - after hours check disabled",
			afterHoursCheck: false,
			timeStr:         "2024-01-08T10:00:00-05:00", // Monday 10:00 AM ET
			expectSkip:      false,
		},
		{
			name:            "after hours - after hours check disabled",
			afterHoursCheck: false,
			timeStr:         "2024-01-08T16:00:00-05:00", // Monday 4:00 PM ET
			expectSkip:      true,
		},
		{
			name:            "after hours - after hours check enabled",
			afterHoursCheck: true,
			timeStr:         "2024-01-08T16:00:00-05:00", // Monday 4:00 PM ET
			expectSkip:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Schedule: ScheduleConfig{
					TradingStart:    "09:45",
					TradingEnd:      "15:45",
					Timezone:        "America/New_York",
					AfterHoursCheck: tt.afterHoursCheck,
				},
			}

			testTime, err := time.Parse(time.RFC3339, tt.timeStr)
			if err != nil {
				t.Fatalf("failed to parse test time: %v", err)
			}

			isWithinHours := config.IsWithinTradingHours(testTime)
			shouldSkip := !isWithinHours && !config.Schedule.AfterHoursCheck

			if shouldSkip != tt.expectSkip {
				t.Errorf("shouldSkip = %v, expected %v (isWithinHours: %v, afterHoursCheck: %v)",
					shouldSkip, tt.expectSkip, isWithinHours, config.Schedule.AfterHoursCheck)
			}
		})
	}
}

func TestValidate_StoragePath(t *testing.T) {
	// Create a base valid config
	baseConfig := &Config{
		Environment: EnvironmentConfig{
			Mode:     "paper",
			LogLevel: "info",
		},
		Broker: BrokerConfig{
			Provider:    "tradier",
			APIKey:      "test-key",
			APIEndpoint: "https://sandbox.tradier.com/v1",
			AccountID:   "test-account",
			UseOTOCO:    true,
		},
		Strategy: StrategyConfig{
			Symbol:              "SPY",
			AllocationPct:       0.35,
			EscalateLossPct:     2.0,
			UseMockHistoricalIV: false,
			Entry: EntryConfig{
				MinIVR:    30,
				TargetDTE: 45,
				DTERange:  []int{40, 50},
				Delta:     16,
				MinCredit: 2.00,
			},
			Exit: ExitConfig{
				ProfitTarget: 0.50,
				MaxDTE:       21,
				StopLossPct:  2.5,
			},
			Adjustments: AdjustmentConfig{
				Enabled:             false,
				SecondDownThreshold: 10,
			},
		},
		Risk: RiskConfig{
			MaxContracts:    1,
			MaxDailyLoss:    500,
			MaxPositionLoss: 3.0,
		},
		Schedule: ScheduleConfig{
			MarketCheckInterval: "15m",
			TradingStart:        "09:45",
			TradingEnd:          "15:45",
			AfterHoursCheck:     false,
		},
	}

	t.Run("valid storage path", func(t *testing.T) {
		config := *baseConfig
		config.Storage.Path = "positions.json"

		err := config.Validate()
		if err != nil {
			t.Errorf("Expected valid config with storage path, got error: %v", err)
		}
	})

	t.Run("empty storage path - invalid", func(t *testing.T) {
		config := *baseConfig
		config.Storage.Path = ""

		err := config.Validate()
		if err == nil {
			t.Error("Expected error when storage path is empty")
		}
		expectedMsg := "storage.path is required when persistence is enabled"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %v", expectedMsg, err)
		}
	})

	t.Run("whitespace-only storage path - invalid", func(t *testing.T) {
		config := *baseConfig
		config.Storage.Path = "   "

		err := config.Validate()
		if err == nil {
			t.Error("Expected error when storage path is whitespace-only")
		}
		expectedMsg := "storage.path is required when persistence is enabled"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %v", expectedMsg, err)
		}
	})

	t.Run("valid storage path with spaces", func(t *testing.T) {
		config := *baseConfig
		config.Storage.Path = "my positions.json"

		err := config.Validate()
		if err != nil {
			t.Errorf("Expected valid config with storage path containing spaces, got error: %v", err)
		}
	})
}

func TestNormalizeExitConfig_StopLossClamping(t *testing.T) {
	t.Run("stop_loss_pct clamped when max_position_loss is less than default", func(t *testing.T) {
		config := &Config{
			Strategy: StrategyConfig{
				Exit: ExitConfig{
					StopLossPct: 0, // unset
				},
			},
			Risk: RiskConfig{
				MaxPositionLoss: 2.0, // less than defaultStopLossPct (2.5)
			},
		}

		config.normalizeExitConfig()

		if config.Strategy.Exit.StopLossPct != 2.0 {
			t.Errorf("Expected StopLossPct to be clamped to 2.0, got %.2f", config.Strategy.Exit.StopLossPct)
		}
	})

	t.Run("stop_loss_pct not clamped when max_position_loss is greater than default", func(t *testing.T) {
		config := &Config{
			Strategy: StrategyConfig{
				Exit: ExitConfig{
					StopLossPct: 0, // unset
				},
			},
			Risk: RiskConfig{
				MaxPositionLoss: 3.0, // greater than defaultStopLossPct (2.5)
			},
		}

		config.normalizeExitConfig()

		if config.Strategy.Exit.StopLossPct != 2.5 {
			t.Errorf("Expected StopLossPct to be 2.5, got %.2f", config.Strategy.Exit.StopLossPct)
		}
	})

	t.Run("explicit stop_loss_pct remains unchanged", func(t *testing.T) {
		config := &Config{
			Strategy: StrategyConfig{
				Exit: ExitConfig{
					StopLossPct: 1.5, // explicitly set
				},
			},
			Risk: RiskConfig{
				MaxPositionLoss: 2.0,
			},
		}

		config.normalizeExitConfig()

		if config.Strategy.Exit.StopLossPct != 1.5 {
			t.Errorf("Expected StopLossPct to remain 1.5, got %.2f", config.Strategy.Exit.StopLossPct)
		}
	})

	t.Run("max_position_loss defaulted when unset", func(t *testing.T) {
		config := &Config{
			Risk: RiskConfig{
				MaxPositionLoss: 0, // unset
			},
		}

		config.normalizeExitConfig()

		if config.Risk.MaxPositionLoss != 3.0 {
			t.Errorf("Expected MaxPositionLoss to be defaulted to 3.0, got %.2f", config.Risk.MaxPositionLoss)
		}
	})
}
