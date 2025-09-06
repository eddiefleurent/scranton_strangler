package storage

import (
	"fmt"

	"github.com/eddiefleurent/scranton_strangler/internal/models"
)

// MockStorage implements Interface for testing
type MockStorage struct {
	saveError       error
	loadError       error
	currentPosition *models.Position
	dailyPnL        map[string]float64
	statistics      *Statistics
	history         []models.Position
	saveCallCount   int
	loadCallCount   int
}

// NewMockStorage creates a new mock storage for testing
func NewMockStorage() *MockStorage {
	return &MockStorage{
		dailyPnL:   make(map[string]float64),
		statistics: &Statistics{},
	}
}

// GetCurrentPosition returns the mock current position.
func (m *MockStorage) GetCurrentPosition() *models.Position {
	if m.currentPosition == nil {
		return nil
	}
	// Return a deep copy to prevent external mutation of internal state
	return clonePosition(m.currentPosition)
}

// SetCurrentPosition updates the mock current position.
func (m *MockStorage) SetCurrentPosition(pos *models.Position) error {
	m.currentPosition = pos
	return nil
}

// ClosePosition closes the mock position.
func (m *MockStorage) ClosePosition(finalPnL float64, reason string) error {
	if m.currentPosition == nil {
		return fmt.Errorf("no position to close")
	}

	// Transition state to closed
	if err := m.currentPosition.TransitionState(models.StateClosed, reason); err != nil {
		return fmt.Errorf("failed to transition position to closed state: %w", err)
	}

	m.currentPosition.CurrentPnL = finalPnL

	// Add to history
	m.history = append(m.history, *m.currentPosition)

	// Update statistics (simplified)
	m.updateStatistics(finalPnL)

	// Clear current position
	m.currentPosition = nil

	return nil
}

// AddAdjustment adds an adjustment to the mock position.
func (m *MockStorage) AddAdjustment(adj models.Adjustment) error {
	if m.currentPosition == nil {
		return fmt.Errorf("no current position to adjust")
	}

	m.currentPosition.Adjustments = append(m.currentPosition.Adjustments, adj)
	return nil
}

// Save simulates saving data (mock implementation).
func (m *MockStorage) Save() error {
	m.saveCallCount++
	return m.saveError
}

// Load simulates loading data (mock implementation).
func (m *MockStorage) Load() error {
	m.loadCallCount++
	return m.loadError
}

// GetHistory returns the mock historical position data.
func (m *MockStorage) GetHistory() []models.Position {
	return m.history
}

// GetStatistics returns the mock statistics data.
func (m *MockStorage) GetStatistics() *Statistics {
	return m.statistics
}

// GetDailyPnL returns the mock daily P&L for a date.
func (m *MockStorage) GetDailyPnL(date string) float64 {
	return m.dailyPnL[date]
}

// SetSaveError configures the mock to return an error on Save calls.
func (m *MockStorage) SetSaveError(err error) {
	m.saveError = err
}

// SetLoadError configures the mock to return an error on Load calls.
func (m *MockStorage) SetLoadError(err error) {
	m.loadError = err
}

// GetSaveCallCount returns the number of times Save was called.
func (m *MockStorage) GetSaveCallCount() int {
	return m.saveCallCount
}

// GetLoadCallCount returns the number of times Load was called.
func (m *MockStorage) GetLoadCallCount() int {
	return m.loadCallCount
}

// AddHistoryPosition adds a position to the mock history.
func (m *MockStorage) AddHistoryPosition(pos models.Position) {
	m.history = append(m.history, pos)
}

// SetDailyPnL sets the mock daily P&L for a specific date.
func (m *MockStorage) SetDailyPnL(date string, pnl float64) {
	m.dailyPnL[date] = pnl
}

// Helper method to update statistics (simplified version)
func (m *MockStorage) updateStatistics(pnl float64) {
	m.statistics.TotalTrades++
	m.statistics.TotalPnL += pnl

	if pnl > 0 {
		m.statistics.WinningTrades++
		if m.statistics.WinningTrades == 1 {
			m.statistics.AverageWin = pnl
		} else {
			m.statistics.AverageWin = (m.statistics.AverageWin*float64(m.statistics.WinningTrades-1) + pnl) /
				float64(m.statistics.WinningTrades)
		}
	} else if pnl < 0 {
		m.statistics.LosingTrades++
		if m.statistics.LosingTrades == 1 {
			m.statistics.AverageLoss = pnl
		} else {
			m.statistics.AverageLoss = (m.statistics.AverageLoss*float64(m.statistics.LosingTrades-1) + pnl) /
				float64(m.statistics.LosingTrades)
		}
	}
	// pnl == 0 is treated as breakeven - don't increment wins or losses

	// Calculate win rate based only on wins and losses (exclude breakevens)
	totalDecidedTrades := m.statistics.WinningTrades + m.statistics.LosingTrades
	if totalDecidedTrades > 0 {
		m.statistics.WinRate = float64(m.statistics.WinningTrades) / float64(totalDecidedTrades) * 100
	}
}

// Ensure MockStorage implements Interface
var _ Interface = (*MockStorage)(nil)
