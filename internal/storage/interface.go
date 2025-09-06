package storage

import "github.com/eddiefleurent/scranton_strangler/internal/models"

// Interface defines the contract for position and trade data persistence
type Interface interface {
	// Position management
	GetCurrentPosition() *models.Position
	SetCurrentPosition(pos *models.Position) error
	ClosePosition(finalPnL float64, reason string) error
	AddAdjustment(adj models.Adjustment) error

	// Data persistence
	Save() error
	Load() error

	// Historical data and analytics
	GetHistory() []models.Position
	GetStatistics() *Statistics
	GetDailyPnL(date string) float64
}

// NewStorage creates a new storage implementation (currently JSON-based)
// In the future, this can be extended to support different storage backends
func NewStorage(filepath string) (Interface, error) {
	return NewJSONStorage(filepath)
}

// Ensure JSONStorage implements Interface
var _ Interface = (*JSONStorage)(nil)
