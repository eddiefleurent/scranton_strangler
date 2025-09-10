package storage

import (
	"fmt"
	"sync"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/models"
)

// MockStorage implements Interface for testing
type MockStorage struct {
	mu               sync.RWMutex
	saveError        error
	loadError        error
	currentPosition  *models.Position
	currentPositions []models.Position
	dailyPnL         map[string]float64
	statistics       *Statistics
	history          []models.Position
	saveCallCount    int
	loadCallCount    int
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
	m.mu.RLock()
	defer m.mu.RUnlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentPosition == nil {
		return fmt.Errorf("no position to close")
	}

	// Map the reason to appropriate state transition condition (consistent with JSONStorage)
	var condition string
	currentState := m.currentPosition.GetCurrentState()
	switch currentState {
	case models.StateOpen:
		condition = models.ConditionPositionClosed
	case models.StateSubmitted:
		condition = models.ConditionOrderTimeout
	case models.StateFirstDown, models.StateSecondDown:
		condition = models.ConditionExitConditions
	case models.StateThirdDown:
		condition = models.ConditionHardStop
	case models.StateFourthDown:
		condition = models.ConditionEmergencyExit
	case models.StateError:
		condition = models.ConditionForceClose
	case models.StateAdjusting:
		condition = models.ConditionHardStop
	case models.StateRolling:
		condition = models.ConditionForceClose
	default:
		condition = models.ConditionExitConditions // fallback
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
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentPosition == nil {
		return fmt.Errorf("no current position to adjust")
	}

	m.currentPosition.Adjustments = append(m.currentPosition.Adjustments, adj)
	return nil
}

// Save simulates saving data (mock implementation).
func (m *MockStorage) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.saveCallCount++
	return m.saveError
}

// Load simulates loading data (mock implementation).
func (m *MockStorage) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.loadCallCount++
	return m.loadError
}

// GetHistory returns the mock historical position data.
func (m *MockStorage) GetHistory() []models.Position {
	m.mu.RLock()
	defer m.mu.RUnlock()

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
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, pos := range m.history {
		if pos.ID == id {
			return true
		}
	}
	return false
}

// GetStatistics returns the mock statistics data.
func (m *MockStorage) GetStatistics() *Statistics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.statistics == nil {
		return &Statistics{}
	}
	s := *m.statistics
	return &s
}

// GetDailyPnL returns the mock daily P&L for a date.
func (m *MockStorage) GetDailyPnL(date string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.dailyPnL[date]
}

// SetSaveError configures the mock to return an error on Save calls.
func (m *MockStorage) SetSaveError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.saveError = err
}

// SetLoadError configures the mock to return an error on Load calls.
func (m *MockStorage) SetLoadError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.loadError = err
}

// GetSaveCallCount returns the number of times Save was called.
func (m *MockStorage) GetSaveCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.saveCallCount
}

// GetLoadCallCount returns the number of times Load was called.
func (m *MockStorage) GetLoadCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.loadCallCount
}

// AddHistoryPosition adds a position to the mock history.
func (m *MockStorage) AddHistoryPosition(pos models.Position) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cp := clonePosition(&pos); cp != nil {
		m.history = append(m.history, *cp)
		return
	}
	m.history = append(m.history, pos)
}

// SetDailyPnL sets the mock daily P&L for a specific date.
func (m *MockStorage) SetDailyPnL(date string, pnl float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dailyPnL[date] = pnl
}

// Helper method to update statistics (consistent with JSONStorage)
func (m *MockStorage) updateStatistics(pnl float64) {
	// Note: this method assumes caller has already acquired the mutex
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
			m.statistics.AverageLoss = -pnl
		} else {
			m.statistics.AverageLoss = (m.statistics.AverageLoss*float64(m.statistics.LosingTrades-1) + (-pnl)) /
				float64(m.statistics.LosingTrades)
		}
	} else {
		// breakeven: do not change win/loss counts or streak
		m.statistics.BreakEvenTrades++
	}

	// Calculate win rate over decided trades (excluding breakevens)
	decided := m.statistics.WinningTrades + m.statistics.LosingTrades
	if decided > 0 {
		m.statistics.WinRate = float64(m.statistics.WinningTrades) / float64(decided)
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
	return nil, fmt.Errorf("%w for symbol %s", ErrNoIVReadings, symbol)
}

// GetCurrentPositions returns all current open positions
func (m *MockStorage) GetCurrentPositions() []models.Position {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Migrate legacy single position if needed
	if m.currentPosition != nil && len(m.currentPositions) == 0 {
		m.currentPositions = []models.Position{*m.currentPosition}
	}

	// Return a deep copy
	positions := make([]models.Position, len(m.currentPositions))
	for i := range m.currentPositions {
		cloned := clonePosition(&m.currentPositions[i])
		if cloned != nil {
			positions[i] = *cloned
		}
	}
	return positions
}

// AddPosition adds a new position to the current positions list
func (m *MockStorage) AddPosition(pos *models.Position) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pos == nil {
		return fmt.Errorf("position cannot be nil")
	}

	// Check if position already exists
	for _, existingPos := range m.currentPositions {
		if existingPos.ID == pos.ID {
			return fmt.Errorf("position with ID %s already exists", pos.ID)
		}
	}

	// Create a deep copy to avoid storing caller pointer directly
	cloned := clonePosition(pos)
	if cloned == nil {
		return fmt.Errorf("failed to clone position for storage")
	}

	// Append the cloned copy to currentPositions
	m.currentPositions = append(m.currentPositions, *cloned)

	// Set currentPosition to point to the same cloned instance (not the caller's pointer)
	m.currentPosition = cloned

	return nil
}

// UpdatePosition updates an existing position
func (m *MockStorage) UpdatePosition(pos *models.Position) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pos == nil {
		return fmt.Errorf("position cannot be nil")
	}

	found := false
	for i := range m.currentPositions {
		if m.currentPositions[i].ID == pos.ID {
			// Create a deep copy to avoid aliasing with caller's struct
			cloned := clonePosition(pos)
			if cloned == nil {
				return fmt.Errorf("failed to clone position for update")
			}

			// Update the position in the slice with the cloned copy
			m.currentPositions[i] = *cloned

			// Update legacy single position with the same cloned instance
			m.currentPosition = cloned
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("position with ID %s not found", pos.ID)
	}

	return nil
}

// GetPositionByID retrieves a specific position by ID
func (m *MockStorage) GetPositionByID(id string) *models.Position {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.currentPositions {
		if m.currentPositions[i].ID == id {
			return clonePosition(&m.currentPositions[i])
		}
	}

	// Check legacy single position
	if m.currentPosition != nil && m.currentPosition.ID == id {
		return clonePosition(m.currentPosition)
	}

	return nil
}

// ClosePositionByID closes a specific position by ID
func (m *MockStorage) ClosePositionByID(id string, finalPnL float64, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var posToClose *models.Position
	var newPositions []models.Position

	// Find and remove the position
	for i := range m.currentPositions {
		if m.currentPositions[i].ID == id {
			posToClose = &m.currentPositions[i]
		} else {
			newPositions = append(newPositions, m.currentPositions[i])
		}
	}

	if posToClose == nil {
		// Check legacy single position
		if m.currentPosition != nil && m.currentPosition.ID == id {
			posToClose = m.currentPosition
			m.currentPosition = nil
		} else {
			return fmt.Errorf("position with ID %s not found", id)
		}
	}

	// Determine canonical condition based on current state
	var condition string
	switch posToClose.GetCurrentState() {
	case models.StateOpen:
		condition = models.ConditionPositionClosed
	case models.StateSubmitted:
		condition = models.ConditionOrderTimeout
	case models.StateFirstDown, models.StateSecondDown:
		condition = models.ConditionExitConditions
	case models.StateThirdDown:
		condition = models.ConditionHardStop
	case models.StateFourthDown:
		condition = models.ConditionEmergencyExit
	case models.StateAdjusting:
		condition = models.ConditionHardStop
	case models.StateRolling:
		condition = models.ConditionForceClose
	case models.StateError:
		condition = models.ConditionForceClose
	default:
		condition = models.ConditionExitConditions
	}

	if err := posToClose.TransitionState(models.StateClosed, condition); err != nil {
		return fmt.Errorf("failed to transition to closed: %w", err)
	}

	posToClose.CurrentPnL = finalPnL
	posToClose.ExitReason = reason

	// Update positions list
	m.currentPositions = newPositions

	// Clear legacy single position if it matches
	if m.currentPosition != nil && m.currentPosition.ID == id {
		m.currentPosition = nil
	}

	// Add to history (copy)
	m.history = append(m.history, *posToClose)

	// Update statistics via shared helper
	m.updateStatistics(finalPnL)

	// Update daily P&L
	dateStr := time.Now().Format("2006-01-02")
	m.dailyPnL[dateStr] = m.dailyPnL[dateStr] + finalPnL

	return nil
}

// Ensure MockStorage implements Interface
var _ Interface = (*MockStorage)(nil)
