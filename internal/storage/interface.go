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
	GetCurrentPositions() []models.Position
	AddPosition(pos *models.Position) error
	UpdatePosition(pos *models.Position) error
	ClosePositionByID(id string, finalPnL float64, reason string) error
	// GetPositionByID retrieves a position by ID, returning a copy to ensure thread safety.
	// Returns (position, true) if found, or (zero-value, false) if not found.
	// The returned position is a deep copy - callers can safely modify it without affecting storage.
	GetPositionByID(id string) (models.Position, bool)
	// DeletePosition removes a position from storage without state machine transitions.
	// Used for cleaning up phantom/invalid positions that never properly entered the system.
	// Does not move position to history - it's simply removed.
	DeletePosition(id string) error

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
