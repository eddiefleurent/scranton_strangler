package storage

import "github.com/eddie/spy-strangle-bot/internal/models"

// StorageInterface defines the contract for position and trade data persistence
type StorageInterface interface {
	// Position management
	GetCurrentPosition() *models.Position
	SetCurrentPosition(pos *models.Position) error
	ClosePosition(finalPnL float64) error
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
func NewStorage(filepath string) (StorageInterface, error) {
	return NewJSONStorage(filepath)
}

// Ensure JSONStorage implements StorageInterface
var _ StorageInterface = (*JSONStorage)(nil)