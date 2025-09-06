package config

import (
	"path/filepath"
	"strings"
	"testing"
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
		config.Strategy.EscalateLossPct = 2.0
		config.Strategy.Exit.StopLossPct = 2.5
		config.Risk.MaxPositionLoss = 2.0

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

	t.Run("stop_loss_pct less than max_position_loss - invalid", func(t *testing.T) {
		config := *baseConfig
		config.Strategy.EscalateLossPct = 1.5
		config.Strategy.Exit.StopLossPct = 2.0
		config.Risk.MaxPositionLoss = 2.5

		err := config.Validate()
		if err == nil {
			t.Error("Expected error when stop_loss_pct < max_position_loss")
		}
		expectedMsg := "strategy.exit.stop_loss_pct (2.00) should be >= risk.max_position_loss (2.50)"
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
		config.Strategy.Exit.StopLossPct = 2.0
		config.Risk.MaxPositionLoss = 1.5

		err := config.Validate()
		if err != nil {
			t.Errorf("Expected valid config with proper ascending order, got error: %v", err)
		}
	})
}
