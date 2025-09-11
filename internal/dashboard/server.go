package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/http"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sirupsen/logrus"
)

//go:embed web/templates/*
var templateFS embed.FS

//go:embed web/static/*
var staticFS embed.FS

type Server struct {
	router              *chi.Mux
	server              *http.Server
	storage             storage.Interface
	broker              broker.Broker
	logger              *logrus.Logger
	port                int
	authToken           string
	allocationThreshold float64
	profitTarget        float64
	stopLossPct         float64
}

type Config struct {
	Port                int
	AuthToken           string
	AllocationThreshold float64 // Allocation threshold percentage (0-100)
	ProfitTarget        float64 // Strategy profit target (0-1, e.g., 0.5 for 50%)
	StopLossPct         float64 // Strategy stop loss percentage (e.g., 2.5 for 250%)
}

type DashboardData struct {
	Positions      []PositionView
	Stats          Statistics
	LastUpdate     time.Time
	AccountBalance float64
	MarketStatus   string
}

type PositionView struct {
	ID               string
	Symbol           string
	State            string
	EntryDate        time.Time
	DTE              int
	CallStrike       float64
	PutStrike        float64
	CreditReceived   float64
	CurrentPnL       float64
	PnLPercent       float64
	ProfitTarget     float64
	StopLoss         float64
	RiskLevelPercent float64
	IsProfit         bool
}

type Statistics struct {
	TotalTrades         int
	WinningTrades       int
	LosingTrades        int
	WinRate             float64
	TotalPnL            float64
	AveragePnL          float64
	CurrentOpen         int
	TotalAllocated      float64
	AllocationPct       float64
	AllocationThreshold float64
	IsAllocationHigh    bool
}

func NewServer(cfg Config, storage storage.Interface, broker broker.Broker, logger *logrus.Logger) *Server {
	s := &Server{
		router:              chi.NewRouter(),
		storage:             storage,
		broker:              broker,
		logger:              logger,
		port:                cfg.Port,
		authToken:           cfg.AuthToken,
		allocationThreshold: cfg.AllocationThreshold,
		profitTarget:        cfg.ProfitTarget,
		stopLossPct:         cfg.StopLossPct,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(60 * time.Second))

	if s.authToken != "" {
		s.router.Use(s.authMiddleware)
	}

	// Create a filesystem rooted at the "static" directory for proper embedded filesystem serving
	sub, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		s.logger.WithError(err).Fatal("Failed to create static filesystem")
	}
	s.router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))

	s.router.Get("/", s.handleDashboard)
	s.router.Get("/api/positions", s.handleGetPositions)
	s.router.Get("/api/stats", s.handleGetStats)
	s.router.Get("/api/position/{id}", s.handleGetPosition)
	s.router.Get("/health", s.handleHealth)

	s.router.Get("/partials/positions", s.handlePositionsPartial)
	s.router.Get("/partials/stats", s.handleStatsPartial)
	s.router.Get("/partials/position/{id}", s.handlePositionDetailPartial)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get("X-Auth-Token")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token != s.authToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:           fmt.Sprintf(":%d", s.port),
		Handler:        s.router,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	s.logger.Infof("Starting dashboard server on port %d", s.port)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 { return a * b },
		"div": func(a, b float64) float64 { 
			if b == 0 { return 0 } // Prevent division by zero
			return a / b 
		},
	}
	tmpl, err := template.New("dashboard.html").Funcs(funcMap).ParseFS(templateFS, "web/templates/dashboard.html")
	if err != nil {
		s.logger.WithError(err).Error("Failed to parse dashboard template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data, err := s.getDashboardData()
	if err != nil {
		s.logger.WithError(err).Error("Failed to get dashboard data")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		s.logger.WithError(err).Error("Failed to execute dashboard template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) handleGetPositions(w http.ResponseWriter, r *http.Request) {
	positions := s.storage.GetCurrentPositions()

	views := s.convertPositionsToViews(positions)
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(views); err != nil {
		s.logger.WithError(err).Error("Failed to encode positions")
	}
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.calculateStatistics()
	if err != nil {
		s.logger.WithError(err).Error("Failed to calculate statistics")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		s.logger.WithError(err).Error("Failed to encode statistics")
	}
}

func (s *Server) handleGetPosition(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	
	position, found := s.storage.GetPositionByID(id)
	if !found {
		s.logger.WithField("position_id", id).Error("Position not found")
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	view := s.convertPositionToView(&position)
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		s.logger.WithError(err).Error("Failed to encode position")
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		s.logger.WithError(err).Error("Failed to encode health response")
	}
}

func (s *Server) handlePositionsPartial(w http.ResponseWriter, r *http.Request) {
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 { return a * b },
		"div": func(a, b float64) float64 { 
			if b == 0 { return 0 } // Prevent division by zero
			return a / b 
		},
	}
	tmpl, err := template.New("positions.html").Funcs(funcMap).ParseFS(templateFS, "web/templates/positions.html")
	if err != nil {
		s.logger.WithError(err).Error("Failed to parse positions template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	positions := s.storage.GetCurrentPositions()

	views := s.convertPositionsToViews(positions)
	
	if err := tmpl.Execute(w, views); err != nil {
		s.logger.WithError(err).Error("Failed to execute positions template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) handleStatsPartial(w http.ResponseWriter, r *http.Request) {
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 { return a * b },
		"div": func(a, b float64) float64 { 
			if b == 0 { return 0 } // Prevent division by zero
			return a / b 
		},
	}
	tmpl, err := template.New("stats.html").Funcs(funcMap).ParseFS(templateFS, "web/templates/stats.html")
	if err != nil {
		s.logger.WithError(err).Error("Failed to parse stats template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	stats, err := s.calculateStatistics()
	if err != nil {
		s.logger.WithError(err).Error("Failed to calculate statistics")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, stats); err != nil {
		s.logger.WithError(err).Error("Failed to execute stats template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) handlePositionDetailPartial(w http.ResponseWriter, r *http.Request) {
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 { return a * b },
		"div": func(a, b float64) float64 { 
			if b == 0 { return 0 } // Prevent division by zero
			return a / b 
		},
	}
	tmpl, err := template.New("position-detail.html").Funcs(funcMap).ParseFS(templateFS, "web/templates/position-detail.html")
	if err != nil {
		s.logger.WithError(err).Error("Failed to parse position detail template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	id := chi.URLParam(r, "id")
	position, found := s.storage.GetPositionByID(id)
	if !found {
		s.logger.WithField("position_id", id).Error("Position not found")
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	view := s.convertPositionToView(&position)
	
	if err := tmpl.Execute(w, view); err != nil {
		s.logger.WithError(err).Error("Failed to execute position detail template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) getDashboardData() (*DashboardData, error) {
	positions := s.storage.GetCurrentPositions()

	stats, err := s.calculateStatistics()
	if err != nil {
		return nil, err
	}

	accountBalance, err := s.broker.GetAccountBalance()
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get account balance")
		accountBalance = 0
	}

	marketStatus := "Closed"
	if isMarketOpen() {
		marketStatus = "Open"
	}

	return &DashboardData{
		Positions:      s.convertPositionsToViews(positions),
		Stats:          *stats,
		LastUpdate:     time.Now(),
		AccountBalance: accountBalance,
		MarketStatus:   marketStatus,
	}, nil
}

func (s *Server) convertPositionsToViews(positions []models.Position) []PositionView {
	views := make([]PositionView, 0, len(positions))
	
	for _, pos := range positions {
		if pos.State == models.StateClosed {
			continue
		}
		views = append(views, s.convertPositionToView(&pos))
	}
	
	return views
}

func (s *Server) convertPositionToView(pos *models.Position) PositionView {
	dte := int(time.Until(pos.Expiration).Hours() / 24)
	if dte < 0 {
		dte = 0
	}
	
	currentPnL := pos.CurrentPnL
	pnlPercent := 0.0
	if pos.CreditReceived > 0 {
		pnlPercent = (currentPnL / pos.CreditReceived) * 100
	}

	profitTarget := pos.CreditReceived * s.profitTarget
	stopLoss := pos.CreditReceived * -s.stopLossPct
	
	// Calculate risk level percentage
	riskLevelPercent := 0.0
	if currentPnL < 0 && stopLoss < 0 {
		riskLevelPercent = (math.Abs(currentPnL) / math.Abs(stopLoss)) * 100
		if riskLevelPercent > 100 {
			riskLevelPercent = 100
		}
	}

	return PositionView{
		ID:               pos.ID,
		Symbol:           pos.Symbol,
		State:            string(pos.State),
		EntryDate:        pos.EntryDate,
		DTE:              dte,
		CallStrike:       pos.CallStrike,
		PutStrike:        pos.PutStrike,
		CreditReceived:   pos.CreditReceived,
		CurrentPnL:       currentPnL,
		PnLPercent:       pnlPercent,
		ProfitTarget:     profitTarget,
		StopLoss:         stopLoss,
		RiskLevelPercent: riskLevelPercent,
		IsProfit:         currentPnL > 0,
	}
}

func (s *Server) calculateStatistics() (*Statistics, error) {
	positions := s.storage.GetCurrentPositions()
	historicalPositions := s.storage.GetHistory()

	stats := &Statistics{}
	var totalAllocated float64
	
	// Count current open positions (skip closed positions)
	for _, pos := range positions {
		if pos.State == models.StateClosed {
			continue
		}
		stats.CurrentOpen++
		totalAllocated += pos.CreditReceived * 100
	}
	
	// Count closed positions from history
	for _, pos := range historicalPositions {
		stats.TotalTrades++
		if pos.CurrentPnL > 0 {
			stats.WinningTrades++
		} else {
			stats.LosingTrades++
		}
		stats.TotalPnL += pos.CurrentPnL
	}

	if stats.TotalTrades > 0 {
		stats.WinRate = float64(stats.WinningTrades) / float64(stats.TotalTrades) * 100
		stats.AveragePnL = stats.TotalPnL / float64(stats.TotalTrades)
	}

	accountBalance, err := s.broker.GetAccountBalance()
	if err == nil && accountBalance > 0 {
		stats.TotalAllocated = totalAllocated
		stats.AllocationPct = (totalAllocated / accountBalance) * 100
	}

	// Set allocation threshold and warning flag
	stats.AllocationThreshold = s.allocationThreshold
	stats.IsAllocationHigh = stats.AllocationPct > s.allocationThreshold

	return stats, nil
}

func isMarketOpen() bool {
	now := time.Now()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		// Fallback to UTC if timezone loading fails
		loc = time.UTC
	}
	nyTime := now.In(loc)
	
	if nyTime.Weekday() == time.Saturday || nyTime.Weekday() == time.Sunday {
		return false
	}
	
	hour := nyTime.Hour()
	minute := nyTime.Minute()
	totalMinutes := hour*60 + minute
	
	marketOpen := 9*60 + 30
	marketClose := 16 * 60
	
	return totalMinutes >= marketOpen && totalMinutes < marketClose
}