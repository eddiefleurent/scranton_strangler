// Package storage provides position and trading data persistence functionality.
package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/models"
)

// JSONStorage implements StorageInterface using JSON file persistence
type JSONStorage struct {
	data     *StorageData
	filepath string
	mu       sync.RWMutex
}

// StorageData represents the complete data structure stored in JSON files.
type StorageData struct {
	LastUpdated     time.Time          `json:"last_updated"`
	CurrentPosition *models.Position   `json:"current_position"`
	DailyPnL        map[string]float64 `json:"daily_pnl"`
	Statistics      *Statistics        `json:"statistics"`
	History         []models.Position  `json:"history"`
}

// Statistics represents performance metrics and analytics data.
type Statistics struct {
	TotalTrades   int     `json:"total_trades"`
	WinningTrades int     `json:"winning_trades"`
	LosingTrades  int     `json:"losing_trades"`
	WinRate       float64 `json:"win_rate"`
	TotalPnL      float64 `json:"total_pnl"`
	AverageWin    float64 `json:"average_win"`
	AverageLoss   float64 `json:"average_loss"`
	MaxDrawdown   float64 `json:"max_drawdown"`
	CurrentStreak int     `json:"current_streak"`
}

// NewJSONStorage creates a new JSON-based storage implementation
func NewJSONStorage(filepath string) (*JSONStorage, error) {
	s := &JSONStorage{
		filepath: filepath,
		data: &StorageData{
			DailyPnL:   make(map[string]float64),
			Statistics: &Statistics{},
		},
	}

	// Load existing data if file exists; fail on unexpected errors
	if _, err := os.Stat(filepath); err == nil {
		if loadErr := s.Load(); loadErr != nil {
			return nil, fmt.Errorf("loading storage: %w", loadErr)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat storage file: %w", err)
	}

	return s, nil
}

// Load reads position data from the JSON file.
func (s *JSONStorage) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filepath)
	if err != nil {
		return err
	}

	var loaded StorageData
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	s.data = &loaded

	if s.data == nil {
		s.data = &StorageData{}
	}
	if s.data.Statistics == nil {
		s.data.Statistics = &Statistics{}
	}
	if s.data.DailyPnL == nil {
		s.data.DailyPnL = make(map[string]float64)
	}

	return nil
}

// Save writes position data to the JSON file.
func (s *JSONStorage) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnsafe()
}

// saveUnsafe performs the actual save operation without acquiring locks
// Must be called with mutex already held
func (s *JSONStorage) saveUnsafe() error {
	s.data.LastUpdated = time.Now()

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	// Create temp file in the same directory as the target file to avoid EXDEV
	dir := filepath.Dir(s.filepath)
	f, err := os.CreateTemp(dir, "storage-*")
	if err != nil {
		return err
	}
	tmpFile := f.Name()

	// Ensure cleanup happens even if we return early
	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				// Error already being handled, just log if needed
				_ = err
			}
		}
		if tmpFile != "" {
			if err := os.Remove(tmpFile); err != nil {
				// Error already being handled, just log if needed
				_ = err
			}
		}
	}()

	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		f = nil // Prevent double close in defer
		return err
	}
	f = nil // Prevent close in defer since we closed successfully

	// Try atomic rename first
	if err := os.Rename(tmpFile, s.filepath); err != nil {
		// Check if it's an EXDEV error (cross-device link)
		if linkErr, ok := err.(*os.LinkError); ok && linkErr.Err == syscall.EXDEV {
			// Handle EXDEV by copying the temp file to destination
			if copyErr := s.copyFile(tmpFile, s.filepath); copyErr != nil {
				return fmt.Errorf("failed to copy temp file: %w", copyErr)
			}
		} else {
			return fmt.Errorf("failed to rename temp file: %w", err)
		}
	}

	// Clear tmpFile so defer doesn't try to remove it
	tmpFile = ""

	// Sync the parent directory to ensure directory entry is persisted
	if err := s.syncParentDir(); err != nil {
		return fmt.Errorf("failed to sync parent directory: %w", err)
	}

	return nil
}

// copyFile copies the contents of src to dst, then fsyncs dst
func (s *JSONStorage) copyFile(src, dst string) error {
	// Validate paths to prevent directory traversal attacks
	if err := s.validateFilePath(src); err != nil {
		return fmt.Errorf("invalid source path: %w", err)
	}
	if err := s.validateFilePath(dst); err != nil {
		return fmt.Errorf("invalid destination path: %w", err)
	}

	srcFile, err := os.Open(src) // #nosec G304 - paths are validated above
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := srcFile.Close(); closeErr != nil {
			// Error already being handled, just log if needed
			_ = closeErr
		}
	}()

	dstFile, err := os.Create(dst) // #nosec G304 - paths are validated above
	if err != nil {
		return err
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			// Error already being handled, just log if needed
			_ = err
		}
	}()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// Sync the destination file
	if err := dstFile.Sync(); err != nil {
		return err
	}

	return nil
}

// validateFilePath ensures the file path is safe and within expected bounds
func (s *JSONStorage) validateFilePath(path string) error {
	// Clean the path to resolve any .. or . components
	cleanPath := filepath.Clean(path)

	// Ensure the path is absolute or relative to current directory
	// For security, we don't allow paths that go outside the intended directory structure
	if filepath.IsAbs(cleanPath) {
		// For absolute paths, ensure they don't contain suspicious patterns
		if strings.Contains(cleanPath, "..") {
			return fmt.Errorf("path contains directory traversal: %s", cleanPath)
		}
	} else {
		// For relative paths, resolve them and check
		absPath, err := filepath.Abs(cleanPath)
		if err != nil {
			return fmt.Errorf("failed to resolve absolute path: %w", err)
		}
		if strings.Contains(absPath, "..") {
			return fmt.Errorf("path contains directory traversal: %s", absPath)
		}
	}

	return nil
}

// syncParentDir opens the parent directory of s.filepath and calls Sync on it
func (s *JSONStorage) syncParentDir() error {
	parentDir := filepath.Dir(s.filepath)

	// Validate parent directory path
	if err := s.validateFilePath(parentDir); err != nil {
		return fmt.Errorf("invalid parent directory path: %w", err)
	}

	dir, err := os.Open(parentDir) // #nosec G304 - path is validated above
	if err != nil {
		return err
	}
	defer func() {
		if err := dir.Close(); err != nil {
			// Error already being handled, just log if needed
			_ = err
		}
	}()

	if err := dir.Sync(); err != nil {
		return err
	}

	return nil
}

// GetCurrentPosition returns the currently active position or nil if none exists.
func (s *JSONStorage) GetCurrentPosition() *models.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.CurrentPosition == nil {
		return nil
	}

	// Create a new Position and copy all primitive fields
	pos := &models.Position{
		ID:             s.data.CurrentPosition.ID,
		Symbol:         s.data.CurrentPosition.Symbol,
		PutStrike:      s.data.CurrentPosition.PutStrike,
		CallStrike:     s.data.CurrentPosition.CallStrike,
		Expiration:     s.data.CurrentPosition.Expiration,
		Quantity:       s.data.CurrentPosition.Quantity,
		CreditReceived: s.data.CurrentPosition.CreditReceived,
		EntryDate:      s.data.CurrentPosition.EntryDate,
		EntryIVR:       s.data.CurrentPosition.EntryIVR,
		EntrySpot:      s.data.CurrentPosition.EntrySpot,
		CurrentPnL:     s.data.CurrentPosition.CurrentPnL,
		DTE:            s.data.CurrentPosition.DTE,
	}

	// Deep copy Adjustments slice
	if len(s.data.CurrentPosition.Adjustments) > 0 {
		pos.Adjustments = make([]models.Adjustment, len(s.data.CurrentPosition.Adjustments))
		copy(pos.Adjustments, s.data.CurrentPosition.Adjustments)
	}

	// Deep copy StateMachine
	if s.data.CurrentPosition.StateMachine != nil {
		pos.StateMachine = s.data.CurrentPosition.StateMachine.Copy()
	}

	return pos
}

// SetCurrentPosition updates the current active position in storage.
func (s *JSONStorage) SetCurrentPosition(pos *models.Position) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.CurrentPosition = clonePosition(pos)
	return s.saveUnsafe()
}

// ClosePosition closes the current position and moves it to history.
func (s *JSONStorage) ClosePosition(finalPnL float64, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.CurrentPosition == nil {
		return fmt.Errorf("no position to close")
	}

	// Update position status
	// Position state will be managed by the state machine, not a string field
	if err := s.data.CurrentPosition.TransitionState(models.StateClosed, reason); err != nil {
		return fmt.Errorf("failed to transition position to closed state: %w", err)
	}
	s.data.CurrentPosition.CurrentPnL = finalPnL

	// Add to history
	s.data.History = append(s.data.History, *s.data.CurrentPosition)

	// Update statistics
	s.updateStatistics(finalPnL)

	// Update daily P&L
	today := time.Now().Format("2006-01-02")
	s.data.DailyPnL[today] += finalPnL

	// Clear current position
	s.data.CurrentPosition = nil

	return s.saveUnsafe()
}

func (s *JSONStorage) updateStatistics(pnl float64) {
	stats := s.data.Statistics
	stats.TotalTrades++
	stats.TotalPnL += pnl

	if pnl > 0 {
		stats.WinningTrades++
		if stats.CurrentStreak >= 0 {
			stats.CurrentStreak++
		} else {
			stats.CurrentStreak = 1
		}

		// Update average win
		if stats.WinningTrades > 0 {
			totalWins := stats.AverageWin*float64(stats.WinningTrades-1) + pnl
			stats.AverageWin = totalWins / float64(stats.WinningTrades)
		}
	} else {
		stats.LosingTrades++
		if stats.CurrentStreak <= 0 {
			stats.CurrentStreak--
		} else {
			stats.CurrentStreak = -1
		}

		// Update average loss
		if stats.LosingTrades > 0 {
			totalLosses := stats.AverageLoss*float64(stats.LosingTrades-1) + pnl
			stats.AverageLoss = totalLosses / float64(stats.LosingTrades)
		}
	}

	// Update win rate
	if stats.TotalTrades > 0 {
		stats.WinRate = float64(stats.WinningTrades) / float64(stats.TotalTrades)
	}

	// Update max drawdown
	if pnl < 0 && pnl < stats.MaxDrawdown {
		stats.MaxDrawdown = pnl
	}
}

// GetStatistics calculates and returns performance statistics.
func (s *JSONStorage) GetStatistics() *Statistics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent external mutation of internal state
	stats := *s.data.Statistics
	return &stats
}

// GetDailyPnL returns the profit/loss for a specific date.
func (s *JSONStorage) GetDailyPnL(date string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.DailyPnL[date]
}

// GetHistory returns all historical closed positions.
func (s *JSONStorage) GetHistory() []models.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a deep copy to prevent external mutation of internal state
	history := make([]models.Position, len(s.data.History))
	for i, pos := range s.data.History {
		// Create a deep copy of each position
		cloned := clonePosition(&pos)
		if cloned != nil {
			history[i] = *cloned
		}
	}
	return history
}

// AddAdjustment adds an adjustment to the current position.
func (s *JSONStorage) AddAdjustment(adj models.Adjustment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.CurrentPosition == nil {
		return fmt.Errorf("no position to adjust")
	}

	s.data.CurrentPosition.Adjustments = append(s.data.CurrentPosition.Adjustments, adj)
	return s.saveUnsafe()
}

// clonePosition creates a deep copy of a Position to prevent mutable state leakage
func clonePosition(pos *models.Position) *models.Position {
	if pos == nil {
		return nil
	}

	// Create a new Position and copy all primitive fields
	cloned := &models.Position{
		ID:             pos.ID,
		Symbol:         pos.Symbol,
		PutStrike:      pos.PutStrike,
		CallStrike:     pos.CallStrike,
		Expiration:     pos.Expiration,
		Quantity:       pos.Quantity,
		CreditReceived: pos.CreditReceived,
		EntryDate:      pos.EntryDate,
		EntryIVR:       pos.EntryIVR,
		EntrySpot:      pos.EntrySpot,
		CurrentPnL:     pos.CurrentPnL,
		DTE:            pos.DTE,
	}

	// Deep copy Adjustments slice
	if len(pos.Adjustments) > 0 {
		cloned.Adjustments = make([]models.Adjustment, len(pos.Adjustments))
		copy(cloned.Adjustments, pos.Adjustments)
	}

	// Deep copy StateMachine
	if pos.StateMachine != nil {
		cloned.StateMachine = pos.StateMachine.Copy()
	}

	return cloned
}
