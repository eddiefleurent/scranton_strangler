package storage

import (
	"fmt"
	"time"

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
	cloned := clonePosition(m.currentPosition)
	// For mock storage, don't carry over runtime StateMachine
	cloned.StateMachine = nil
	return cloned
}

// SetCurrentPosition updates the mock current position.
func (m *MockStorage) SetCurrentPosition(pos *models.Position) error {
	cloned := clonePosition(pos)
	// For mock storage, don't carry over runtime StateMachine
	cloned.StateMachine = nil
	m.currentPosition = cloned
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
	return append([]models.Position(nil), m.history...)
}

// HasInHistory checks if a position with the given ID exists in the mock history.
func (m *MockStorage) HasInHistory(id string) bool {
	for _, pos := range m.history {
		if pos.ID == id {
			return true
		}
	}
	return false
}

// GetStatistics returns the mock statistics data.
func (m *MockStorage) GetStatistics() *Statistics {
	if m.statistics == nil {
		return &Statistics{}
	}
	s := *m.statistics
	return &s
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

// StoreIVReading stores a new IV reading (mock implementation)
func (m *MockStorage) StoreIVReading(reading *models.IVReading) error {
	// Mock implementation - just return success
	return nil
}

// GetIVReadings retrieves IV readings within a date range (mock implementation)
func (m *MockStorage) GetIVReadings(symbol string, startDate, endDate time.Time) ([]models.IVReading, error) {
	// Return empty slice for mock
	return []models.IVReading{}, nil
}

// GetLatestIVReading retrieves the most recent IV reading (mock implementation)
func (m *MockStorage) GetLatestIVReading(symbol string) (*models.IVReading, error) {
	// Return nil for mock
	return nil, nil
}

// Ensure MockStorage implements Interface
var _ Interface = (*MockStorage)(nil)
