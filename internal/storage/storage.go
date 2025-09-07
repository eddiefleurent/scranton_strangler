// Package storage provides position and trading data persistence functionality.
package storage

import (
	"encoding/json"
	"errors"
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

// JSONStorage implements Interface using JSON file persistence
type JSONStorage struct {
	data     *Data
	filepath string
	mu       sync.RWMutex
}

// Data represents the complete data structure stored in JSON files.
type Data struct {
	LastUpdated     time.Time          `json:"last_updated"`
	CurrentPosition *models.Position   `json:"current_position"`
	DailyPnL        map[string]float64 `json:"daily_pnl"`
	Statistics      *Statistics        `json:"statistics"`
	History         []models.Position  `json:"history"`
	IVReadings      []models.IVReading `json:"iv_readings"` // Historical IV data
}

// Statistics represents performance metrics and analytics data.
type Statistics struct {
	TotalTrades        int     `json:"total_trades"`
	WinningTrades      int     `json:"winning_trades"`
	LosingTrades       int     `json:"losing_trades"`
	WinRate            float64 `json:"win_rate"`
	TotalPnL           float64 `json:"total_pnl"`
	AverageWin         float64 `json:"average_win"`
	AverageLoss        float64 `json:"average_loss"`          // Average loss magnitude (positive)
	MaxSingleTradeLoss float64 `json:"max_single_trade_loss"` // Largest single trade loss (negative)
	CurrentStreak      int     `json:"current_streak"`
}

// NewJSONStorage creates a new JSON-based storage implementation
func NewJSONStorage(filePath string) (*JSONStorage, error) {
	s := &JSONStorage{
		filepath: filePath,
		data: &Data{
			DailyPnL:   make(map[string]float64),
			Statistics: &Statistics{},
		},
	}

	// Create parent directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		return nil, fmt.Errorf("creating parent directory: %w", err)
	}

	// Load existing data if file exists; fail on unexpected errors
	if _, err := os.Stat(filePath); err == nil {
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

	var loaded Data
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	s.data = &loaded

	if s.data == nil {
		s.data = &Data{}
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
	s.data.LastUpdated = time.Now().UTC()

	// Create temp file in the same directory as the target file to avoid EXDEV
	dir := filepath.Dir(s.filepath)
	f, err := os.CreateTemp(dir, ".storage-*")
	if err != nil {
		return err
	}
	tmpFile := f.Name()

	// Set restrictive permissions on the temporary file
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("failed to set temp file permissions: %w", err)
	}

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

	// Encode directly to the temp file to reduce memory usage
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s.data); err != nil {
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

	// Track whether directory has been synced to avoid double fsync
	dirSynced := false

	// Try atomic rename first
	if err := os.Rename(tmpFile, s.filepath); err != nil {
		// Check if it's an EXDEV error (cross-device link)
		if linkErr, ok := err.(*os.LinkError); ok && errors.Is(linkErr.Err, syscall.EXDEV) {
			// Handle EXDEV by copying the temp file to destination
			if copyErr := s.copyFile(tmpFile, s.filepath); copyErr != nil {
				return fmt.Errorf("failed to copy temp file: %w", copyErr)
			}
			// copyFile already fsyncs destination directory
			dirSynced = true
		} else {
			return fmt.Errorf("failed to rename temp file: %w", err)
		}
	}

	// Clear tmpFile so defer doesn't try to remove it
	tmpFile = ""

	// Sync the parent directory to ensure directory entry is persisted (only if not already synced)
	if !dirSynced {
		if err := s.syncParentDir(); err != nil {
			return fmt.Errorf("failed to sync parent directory: %w", err)
		}
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

	// Get source file info for preserving mode
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Create temporary file in destination directory for atomic operation
	dstDir := filepath.Dir(dst)
	tmpFile, err := os.CreateTemp(dstDir, ".tmp_*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpFileName := tmpFile.Name()

	// Ensure temp file is cleaned up on error
	var tempFileClosed bool
	defer func() {
		if !tempFileClosed {
			_ = tmpFile.Close()
		}
		if tmpFileName != "" {
			_ = os.Remove(tmpFileName)
		}
	}()

	// Set permissions on temp file
	if err := tmpFile.Chmod(srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to set temp file permissions: %w", err)
	}

	// Copy contents to temp file
	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy to temp file: %w", err)
	}

	// Sync the temp file
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// Close temp file before rename
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tempFileClosed = true

	// Atomically rename temp file to final destination
	if err := os.Rename(tmpFileName, dst); err != nil {
		return fmt.Errorf("failed to rename temp file to destination: %w", err)
	}

	// Sync the destination directory to persist the new entry
	if err := s.validateFilePath(dstDir); err != nil {
		return fmt.Errorf("invalid destination directory path: %w", err)
	}
	// #nosec G304 - path is validated above
	if dir, err := os.Open(dstDir); err == nil {
		defer func() { _ = dir.Close() }()
		if syncErr := dir.Sync(); syncErr != nil {
			return fmt.Errorf("failed to fsync destination directory: %w", syncErr)
		}
	}

	// Clear temp file name since rename succeeded
	tmpFileName = ""

	return nil
}

// validateFilePath ensures the file path is safe and within expected bounds
func (s *JSONStorage) validateFilePath(path string) error {
	// Resolve storage root (directory containing the storage file)
	storageRoot := filepath.Dir(s.filepath)
	storageRootClean := filepath.Clean(storageRoot)
	storageRootAbs, err := filepath.Abs(storageRootClean)
	if err != nil {
		return fmt.Errorf("failed to resolve storage root: %w", err)
	}

	// Resolve symlinks in storage root
	storageRootResolved, err := filepath.EvalSymlinks(storageRootAbs)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks in storage root: %w", err)
	}

	// Clean and resolve the target path to absolute path
	targetClean := filepath.Clean(path)
	targetAbs, err := filepath.Abs(targetClean)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %w", err)
	}

	// Resolve symlinks for target:
	// - If target exists: EvalSymlinks on target.
	// - If target missing: EvalSymlinks on parent, then re-join the base.
	var targetResolved string
	if _, statErr := os.Stat(targetAbs); statErr == nil {
		if resolved, err := filepath.EvalSymlinks(targetAbs); err == nil {
			targetResolved = resolved
		} else {
			return fmt.Errorf("failed to resolve symlinks in target: %w", err)
		}
	} else if os.IsNotExist(statErr) {
		parent := filepath.Dir(targetAbs)
		parentResolved, perr := filepath.EvalSymlinks(parent)
		if perr != nil {
			return fmt.Errorf("failed to resolve symlinks in target parent: %w", perr)
		}
		targetResolved = filepath.Join(parentResolved, filepath.Base(targetAbs))
	} else {
		return fmt.Errorf("failed to stat target path: %w", statErr)
	}

	// Compute the relative path from resolved storage root to resolved target
	relPath, err := filepath.Rel(storageRootResolved, targetResolved)
	if err != nil {
		return fmt.Errorf("failed to compute relative path: %w", err)
	}

	// Check if the relative path escapes the storage directory
	// Reject if relative path equals ".." or starts with ".." + separator
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("path escapes storage directory: %s (resolved to: %s)", path, targetResolved)
	}

	return nil
}

// syncParentDir opens the parent directory of s.filepath and calls Sync on it
func (s *JSONStorage) syncParentDir() error {
	parentDir := filepath.Dir(s.filepath)

	// Skip validation for storage root directory for performance
	// parentDir is always the storage root, so validation is redundant
	dir, err := os.Open(parentDir) // #nosec G304 - path is storage root, validated at construction
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

	return clonePosition(s.data.CurrentPosition)
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
	// Map the reason to appropriate state transition condition
	var condition string
	currentState := s.data.CurrentPosition.GetCurrentState()
	switch currentState {
	case models.StateOpen:
		condition = "position_closed"
	case models.StateFirstDown, models.StateSecondDown, models.StateThirdDown, models.StateFourthDown:
		condition = "exit_conditions"
	case models.StateError:
		condition = "force_close"
	case models.StateAdjusting:
		condition = "hard_stop"
	case models.StateRolling:
		condition = "force_close"
	default:
		condition = "exit_conditions" // fallback
	}

	if err := s.data.CurrentPosition.TransitionState(models.StateClosed, condition); err != nil {
		return fmt.Errorf("failed to transition position to closed state: %w", err)
	}
	s.data.CurrentPosition.CurrentPnL = finalPnL

	// If TransitionState doesn't set these, ensure they are recorded.
	if s.data.CurrentPosition.ExitReason == "" {
		s.data.CurrentPosition.ExitReason = reason
	}
	if s.data.CurrentPosition.ExitDate.IsZero() {
		s.data.CurrentPosition.ExitDate = time.Now().UTC()
	}

	// Add to history
	s.data.History = append(s.data.History, *clonePosition(s.data.CurrentPosition))

	// Update statistics
	s.updateStatistics(finalPnL)

	// Update daily P&L
	closedAt := s.data.CurrentPosition.ExitDate
	if closedAt.IsZero() {
		closedAt = time.Now().UTC()
	}
	day := closedAt.Format("2006-01-02")
	s.data.DailyPnL[day] += finalPnL

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

		// Update average loss (using absolute magnitude)
		if stats.LosingTrades > 0 {
			totalLosses := stats.AverageLoss*float64(stats.LosingTrades-1) + (-pnl) // Use absolute magnitude
			stats.AverageLoss = totalLosses / float64(stats.LosingTrades)
		}
	}

	// Update win rate
	if stats.TotalTrades > 0 {
		stats.WinRate = float64(stats.WinningTrades) / float64(stats.TotalTrades)
	}

	// Update max single trade loss
	if pnl < 0 && pnl < stats.MaxSingleTradeLoss {
		stats.MaxSingleTradeLoss = pnl
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

// HasInHistory checks if a position with the given ID exists in the history.
func (s *JSONStorage) HasInHistory(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, pos := range s.data.History {
		if pos.ID == id {
			return true
		}
	}
	return false
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
		State:          pos.State,
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
		EntryOrderID:   pos.EntryOrderID,
		ExitOrderID:    pos.ExitOrderID,
		ExitReason:     pos.ExitReason,
		ExitDate:       pos.ExitDate,
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

// StoreIVReading stores a new IV reading in the storage
func (s *JSONStorage) StoreIVReading(reading *models.IVReading) error {
	if reading == nil {
		return fmt.Errorf("cannot store nil IV reading")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize IVReadings slice if nil
	if s.data.IVReadings == nil {
		s.data.IVReadings = make([]models.IVReading, 0)
	}

	// Check if reading already exists for this symbol and date
	for i, existing := range s.data.IVReadings {
		sameDay := existing.Date.UTC().Truncate(24 * time.Hour).Equal(
			reading.Date.UTC().Truncate(24 * time.Hour),
		)
		if existing.Symbol == reading.Symbol && sameDay {
			// Update existing reading
			s.data.IVReadings[i] = *reading
			return s.saveUnsafe()
		}
	}

	// Add new reading
	s.data.IVReadings = append(s.data.IVReadings, *reading)
	return s.saveUnsafe()
}

// GetIVReadings retrieves IV readings for a symbol within a date range
func (s *JSONStorage) GetIVReadings(symbol string, startDate, endDate time.Time) ([]models.IVReading, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var readings []models.IVReading
	for _, reading := range s.data.IVReadings {
		if reading.Symbol == symbol &&
			!reading.Date.Before(startDate) &&
			!reading.Date.After(endDate) {
			readings = append(readings, reading)
		}
	}

	return readings, nil
}

// GetLatestIVReading retrieves the most recent IV reading for a symbol
func (s *JSONStorage) GetLatestIVReading(symbol string) (*models.IVReading, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *models.IVReading
	var latestTime time.Time

	for _, reading := range s.data.IVReadings {
		if reading.Symbol == symbol && reading.Timestamp.After(latestTime) {
			readingCopy := reading // Create a copy
			latest = &readingCopy
			latestTime = reading.Timestamp
		}
	}

	if latest == nil {
		return nil, fmt.Errorf("no IV readings found for symbol %s", symbol)
	}

	return latest, nil
}
