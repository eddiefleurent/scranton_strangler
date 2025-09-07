package storage

import (
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/models"
)

// Interface defines the contract for position and trade data persistence.
//
// Implementations must be safe for concurrent use - callers can assume all methods
// are goroutine-safe and can safely call these methods from multiple goroutines.
//
// The provided JSONStorage implementation uses sync.RWMutex to serialize access,
// ensuring all Interface methods are protected for concurrent readers and writers.
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
	HasInHistory(id string) bool
	GetStatistics() *Statistics
	GetDailyPnL(date string) float64

	// IV data storage
	StoreIVReading(reading *models.IVReading) error
	GetIVReadings(symbol string, startDate, endDate time.Time) ([]models.IVReading, error)
	GetLatestIVReading(symbol string) (*models.IVReading, error)
}

// NewStorage creates a new storage implementation (currently JSON-based)
// In the future, this can be extended to support different storage backends
func NewStorage(filepath string) (Interface, error) {
	return NewJSONStorage(filepath)
}

// Ensure JSONStorage implements Interface
var _ Interface = (*JSONStorage)(nil)
