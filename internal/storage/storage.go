package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/eddie/spy-strangle-bot/internal/models"
)

type Storage struct {
	mu       sync.RWMutex
	filepath string
	data     *StorageData
}

type StorageData struct {
	CurrentPosition *models.Position   `json:"current_position"`
	History         []models.Position  `json:"history"`
	DailyPnL        map[string]float64 `json:"daily_pnl"`
	Statistics      *Statistics        `json:"statistics"`
	LastUpdated     time.Time          `json:"last_updated"`
}

type Statistics struct {
	TotalTrades    int     `json:"total_trades"`
	WinningTrades  int     `json:"winning_trades"`
	LosingTrades   int     `json:"losing_trades"`
	WinRate        float64 `json:"win_rate"`
	TotalPnL       float64 `json:"total_pnl"`
	AverageWin     float64 `json:"average_win"`
	AverageLoss    float64 `json:"average_loss"`
	MaxDrawdown    float64 `json:"max_drawdown"`
	CurrentStreak  int     `json:"current_streak"`
}

func NewStorage(filepath string) (*Storage, error) {
	s := &Storage{
		filepath: filepath,
		data:     &StorageData{
			DailyPnL:   make(map[string]float64),
			Statistics: &Statistics{},
		},
	}

	// Load existing data if file exists
	if _, err := os.Stat(filepath); err == nil {
		if err := s.Load(); err != nil {
			return nil, fmt.Errorf("loading storage: %w", err)
		}
	}

	return s, nil
}

func (s *Storage) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filepath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &s.data); err != nil {
		return err
	}

	return nil
}

func (s *Storage) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.LastUpdated = time.Now()

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first
	tmpFile := s.filepath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tmpFile, s.filepath)
}

func (s *Storage) GetCurrentPosition() *models.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.CurrentPosition
}

func (s *Storage) SetCurrentPosition(pos *models.Position) error {
	s.mu.Lock()
	s.data.CurrentPosition = pos
	s.mu.Unlock()
	
	return s.Save()
}

func (s *Storage) ClosePosition(finalPnL float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.CurrentPosition == nil {
		return fmt.Errorf("no position to close")
	}

	// Update position status
	s.data.CurrentPosition.Status = "closed"
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

	return s.Save()
}

func (s *Storage) updateStatistics(pnl float64) {
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
			totalWins := stats.AverageWin * float64(stats.WinningTrades-1) + pnl
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
			totalLosses := stats.AverageLoss * float64(stats.LosingTrades-1) + pnl
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

func (s *Storage) GetStatistics() *Statistics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Statistics
}

func (s *Storage) GetDailyPnL(date string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.DailyPnL[date]
}

func (s *Storage) GetHistory() []models.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.History
}

func (s *Storage) AddAdjustment(adj models.Adjustment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.CurrentPosition == nil {
		return fmt.Errorf("no position to adjust")
	}

	s.data.CurrentPosition.Adjustments = append(s.data.CurrentPosition.Adjustments, adj)
	return s.Save()
}