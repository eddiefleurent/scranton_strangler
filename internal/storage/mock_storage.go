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
	// Preserve StateMachine by copying it if it exists (consistent with JSONStorage)
	if m.currentPosition.StateMachine != nil {
		cloned.StateMachine = m.currentPosition.StateMachine.Copy()
	}
	return cloned
}

// SetCurrentPosition updates the mock current position.
func (m *MockStorage) SetCurrentPosition(pos *models.Position) error {
	if pos == nil {
		m.currentPosition = nil
		return nil
	}
	cloned := clonePosition(pos)
	// Preserve StateMachine by copying it if it exists (consistent with JSONStorage)
	if pos.StateMachine != nil {
		cloned.StateMachine = pos.StateMachine.Copy()
	}
	m.currentPosition = cloned
	return nil
}

// ClosePosition closes the mock position.
func (m *MockStorage) ClosePosition(finalPnL float64, reason string) error {
	if m.currentPosition == nil {
		return fmt.Errorf("no position to close")
	}

	// Map the reason to appropriate state transition condition (consistent with JSONStorage)
	var condition string
	currentState := m.currentPosition.GetCurrentState()
	switch currentState {
	case models.StateOpen:
		condition = ConditionPositionClosed
	case models.StateSubmitted:
		condition = ConditionOrderTimeout
	case models.StateFirstDown, models.StateSecondDown, models.StateThirdDown, models.StateFourthDown:
		condition = ConditionExitConditions
	case models.StateError:
		condition = ConditionForceClose
	case models.StateAdjusting:
		condition = ConditionHardStop
	case models.StateRolling:
		condition = ConditionForceClose
	default:
		condition = ConditionExitConditions // fallback
	}

	// Transition state to closed using canonical condition constant
	if err := m.currentPosition.TransitionState(models.StateClosed, condition); err != nil {
		return fmt.Errorf("failed to transition position to closed state: %w", err)
	}

	m.currentPosition.CurrentPnL = finalPnL

	// Set human-readable reason separately (consistent with JSONStorage)
	m.currentPosition.ExitReason = reason
	// Note: ExitDate is already set by TransitionState() call above

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
	out := make([]models.Position, len(m.history))
	for i := range m.history {
		if cp := clonePosition(&m.history[i]); cp != nil {
			out[i] = *cp
		} else {
			out[i] = m.history[i]
		}
	}
	return out
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
	if cp := clonePosition(&pos); cp != nil {
		m.history = append(m.history, *cp)
		return
	}
	m.history = append(m.history, pos)
}

// SetDailyPnL sets the mock daily P&L for a specific date.
func (m *MockStorage) SetDailyPnL(date string, pnl float64) {
	m.dailyPnL[date] = pnl
}

// Helper method to update statistics (consistent with JSONStorage)
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
	} else {
		// Treat pnl <= 0 as losses (including breakeven pnl == 0) - consistent with JSONStorage
		m.statistics.LosingTrades++
		if m.statistics.LosingTrades == 1 {
			if pnl == 0 {
				// For breakeven, include zero loss in AverageLoss calculation
				m.statistics.AverageLoss = 0
			} else {
				m.statistics.AverageLoss = -pnl
			}
		} else {
			var loss float64
			if pnl == 0 {
				// For breakeven, use zero loss
				loss = 0
			} else {
				loss = -pnl
			}
			m.statistics.AverageLoss = (m.statistics.AverageLoss*float64(m.statistics.LosingTrades-1) + loss) /
				float64(m.statistics.LosingTrades)
		}
	}

	// Calculate win rate as ratio (consistent with JSONStorage, not percentage)
	if m.statistics.TotalTrades > 0 {
		m.statistics.WinRate = float64(m.statistics.WinningTrades) / float64(m.statistics.TotalTrades)
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
	// Return not-found error consistent with JSONStorage
	return nil, fmt.Errorf("no IV readings found for symbol %s", symbol)
}

// Ensure MockStorage implements Interface
var _ Interface = (*MockStorage)(nil)
