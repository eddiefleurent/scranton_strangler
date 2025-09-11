package main

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/google/uuid"
)

// Reconciler handles position synchronization between broker and storage
type Reconciler struct {
	broker  broker.Broker
	storage storage.Interface
	logger  *log.Logger
}

// NewReconciler creates a new position reconciler
func NewReconciler(broker broker.Broker, storage storage.Interface, logger *log.Logger) *Reconciler {
	return &Reconciler{
		broker:  broker,
		storage: storage,
		logger:  logger,
	}
}

// ReconcilePositions detects position mismatches between broker and storage
// It handles three cases:
// 1. Positions in storage but closed in broker (manual closes)
// 2. Positions in broker but missing from storage (timeout-related sync issues)
// 3. Cold start: no storage file but broker positions exist (production recovery)
func (r *Reconciler) ReconcilePositions(storedPositions []models.Position) []models.Position {
	// Get current broker positions
	brokerPositions, err := r.broker.GetPositions()
	if err != nil {
		r.logger.Printf("Failed to get broker positions for reconciliation: %v", err)
		return storedPositions // Return unchanged on error
	}

	r.logger.Printf("Reconciling %d stored positions with %d broker positions",
		len(storedPositions), len(brokerPositions))

	// CRITICAL: Cold start scenario - if no stored positions but broker has positions,
	// this is likely a production restart without positions.json file
	if len(storedPositions) == 0 && len(brokerPositions) > 0 {
		r.logger.Printf("COLD START DETECTED: No stored positions but %d broker positions exist", len(brokerPositions))
		r.logger.Printf("Creating recovery positions for existing broker positions...")
	}

	var activePositions []models.Position

	// First pass: Check stored positions against broker
	for _, position := range storedPositions {
		// Skip already closed positions
		if position.GetCurrentState() == models.StateClosed {
			continue
		}

		// Check if this position still exists in the broker
		isOpenInBroker := r.isPositionOpenInBroker(&position, brokerPositions)

		if !isOpenInBroker {
			r.logger.Printf("Position %s manually closed - updating storage", shortID(position.ID))

			// Calculate final P&L (use current P&L if available, otherwise default to 0/unknown)
			finalPnL := position.CurrentPnL
			if finalPnL == 0 {
				// Broker-close inferred PnL should default to 0/unknown rather than optimistic estimate
				finalPnL = 0
			}

			// Close the position with manual close reason
			if err := r.storage.ClosePositionByID(position.ID, finalPnL, "manual_close"); err != nil {
				r.logger.Printf("Failed to close manually detected position %s: %v", shortID(position.ID), err)
				activePositions = append(activePositions, position) // Keep in list if can't close
				continue
			}

			r.logger.Printf("Position %s closed due to manual intervention. Final P&L: $%.2f",
				shortID(position.ID), finalPnL)
		} else {
			// Position is still active in broker
			activePositions = append(activePositions, position)
		}
	}

	// Second pass: Check for orphaned broker positions (positions in broker but not in storage)
	// This handles the case where orders timed out locally but actually filled
	orphanedStrangles := r.findOrphanedStrangles(brokerPositions, activePositions)
	for _, orphanStrangle := range orphanedStrangles {
		r.logger.Printf("Detected orphaned strangle in broker: Put %.0f / Call %.0f, creating recovery position",
			orphanStrangle.putStrike, orphanStrangle.callStrike)

		// Create a recovery position for this orphaned strangle
		recoveryPos := r.createRecoveryPosition(orphanStrangle)
		if recoveryPos != nil {
			if err := r.storage.AddPosition(recoveryPos); err != nil {
				r.logger.Printf("Failed to add recovery position: %v", err)
			} else {
				activePositions = append(activePositions, *recoveryPos)
				r.logger.Printf("Added recovery position %s for orphaned strangle", shortID(recoveryPos.ID))
			}
		}
	}

	return activePositions
}

// orphanedStrangle represents a strangle position found in broker but not in storage
type orphanedStrangle struct {
	putStrike  float64
	callStrike float64
	expiration string
	quantity   int
	symbol     string
}

// findOrphanedStrangles identifies strangle positions in broker that aren't tracked in storage
func (r *Reconciler) findOrphanedStrangles(brokerPositions []broker.PositionItem, activePositions []models.Position) []orphanedStrangle {
	var orphaned []orphanedStrangle

	// Group broker positions by expiration and identify strangles
	positionsByExp := make(map[string][]broker.PositionItem)
	for _, brokerPos := range brokerPositions {
		// Extract underlying ticker from the option symbol
		underlying := extractUnderlyingFromSymbol(brokerPos.Symbol)
		if underlying != "SPY" { // Only handle SPY options
			continue // Skip non-SPY positions
		}

		// Extract expiration from option symbol
		exp := extractExpirationFromSymbol(brokerPos.Symbol)
		if exp != "" {
			positionsByExp[exp] = append(positionsByExp[exp], brokerPos)
		}
	}

	// For each expiration, look for call/put pairs that form strangles
	for exp, positions := range positionsByExp {
		strangles := identifyStranglesFromPositions(positions, exp)

		// Check if each strangle is already tracked in our active positions
		for _, strangle := range strangles {
			isTracked := false
			for _, activePos := range activePositions {
				if strangleMatches(activePos, strangle) {
					isTracked = true
					break
				}
			}

			if !isTracked {
				orphaned = append(orphaned, strangle)
			}
		}
	}

	return orphaned
}

// createRecoveryPosition creates a position object for an orphaned strangle
func (r *Reconciler) createRecoveryPosition(orphan orphanedStrangle) *models.Position {
	// Parse expiration
	expTime, err := time.Parse("2006-01-02", orphan.expiration)
	if err != nil {
		r.logger.Printf("Failed to parse expiration %s for recovery position", orphan.expiration)
		return nil
	}

	// Generate new position ID
	positionID := uuid.New().String()

	// Create position
	position := models.NewPosition(
		positionID,
		orphan.symbol,
		orphan.putStrike,
		orphan.callStrike,
		expTime,
		orphan.quantity,
	)

	// Set as recovered/reconciled position
	position.EntryDate = time.Now().UTC()
	position.DTE = position.CalculateDTE()

	// Set reasonable defaults (we don't know the actual entry details)
	position.CreditReceived = 0 // We don't know the original credit
	position.EntrySpot = 0      // We don't know the original spot price
	position.EntryIV = 0        // We don't know the original IV

	// Transition to Open state (assume it's already filled)
	if err := position.TransitionState(models.StateOpen, models.ConditionRecoveredPosition); err != nil {
		r.logger.Printf("Failed to set recovery position state: %v", err)
		return nil
	}

	return position
}

// isPositionOpenInBroker checks if a stored position still exists in broker positions
func (r *Reconciler) isPositionOpenInBroker(position *models.Position, brokerPositions []broker.PositionItem) bool {
	// For options positions, check if we have both legs still open
	hasCallLeg := false
	hasCallStrike := false
	hasPutLeg := false
	hasPutStrike := false

	// Check if broker position matches our stored position strikes
	for _, brokerPos := range brokerPositions {
		// Parse option symbol using OPRA format: TICKER[YYMMDD][C/P][STRIKE*1000 padded to 8 digits]
		// SPY option format: SPY240315C00610000 or SPY240315P00500000
		parsedStrike, optionType, err := parseOptionSymbol(brokerPos.Symbol)
		if err != nil {
			continue // Skip invalid symbols (like stock symbols)
		}

		// Extract underlying from option symbol
		underlying := extractUnderlyingFromSymbol(brokerPos.Symbol)
		if underlying != position.Symbol {
			continue // Skip positions for different underlyings
		}

		// Extract expiration from option symbol
		expiration := extractExpirationFromSymbol(brokerPos.Symbol)
		expectedExpiration := position.Expiration.Format("2006-01-02")
		if expiration != expectedExpiration {
			continue // Skip positions with different expirations
		}

		// Check if this matches one of our strangle legs
		if optionType == "C" {
			hasCallLeg = true
			if math.Abs(parsedStrike-position.CallStrike) < 0.01 {
				hasCallStrike = true
			}
		} else if optionType == "P" {
			hasPutLeg = true
			if math.Abs(parsedStrike-position.PutStrike) < 0.01 {
				hasPutStrike = true
			}
		}
	}

	// Position is open if we have both call and put legs with matching strikes
	return hasCallLeg && hasPutLeg && hasCallStrike && hasPutStrike
}

// Helper functions

// extractUnderlyingFromSymbol extracts the underlying ticker from an option symbol
// For OPRA format: SPY240315C00610000 -> "SPY"
// For stock symbols: "SPY" -> "SPY"
func extractUnderlyingFromSymbol(symbol string) string {
	// For option symbols, extract the ticker part before the date
	// OPRA format: TICKER[YYMMDD][C/P][STRIKE]
	// Scan for a run of six consecutive digits (YYMMDD) to detect the option date
	
	// Only iterate while there are at least 6 characters remaining to check
	for i := 0; i <= len(symbol)-6; i++ {
		// Check if we have 6 consecutive digits starting at position i
		allDigits := true
		for j := i; j < i+6; j++ {
			if symbol[j] < '0' || symbol[j] > '9' {
				allDigits = false
				break
			}
		}
		
		// If we found 6 consecutive digits and they're not at the start (index > 0),
		// return the substring before them as the underlying ticker
		if allDigits && i > 0 {
			return symbol[:i]
		}
	}
	
	// Fallback: if no 6-digit sequence was found, return the whole symbol
	// (likely a stock symbol, not an option)
	return symbol
}

func extractExpirationFromSymbol(symbol string) string {
	// Option format: TICKER[YYMMDD][C/P][STRIKE]
	// Search for the first occurrence of six consecutive digits (YYMMDD)
	if len(symbol) < 6 {
		return ""
	}

	// Look for 6 consecutive digits
	for i := 0; i <= len(symbol)-6; i++ {
		// Check if we have 6 consecutive digits starting at position i
		allDigits := true
		for j := i; j < i+6; j++ {
			if symbol[j] < '0' || symbol[j] > '9' {
				allDigits = false
				break
			}
		}
		
		if allDigits {
			// Found 6-digit date, extract and format
			dateStr := symbol[i : i+6]
			year := "20" + dateStr[0:2]
			month := dateStr[2:4]
			day := dateStr[4:6]
			return year + "-" + month + "-" + day
		}
	}
	
	// No 6-digit run found
	return ""
}

func identifyStranglesFromPositions(positions []broker.PositionItem, expiration string) []orphanedStrangle {
	var strangles []orphanedStrangle

	// Group positions by strike for this expiration
	callStrikes := make(map[float64]int) // strike -> quantity
	putStrikes := make(map[float64]int)

	for _, pos := range positions {
		strike, optionType, err := parseOptionSymbol(pos.Symbol)
		if err != nil {
			continue
		}

		quantity := int(math.Abs(pos.Quantity)) // Use absolute value

		if optionType == "C" {
			callStrikes[strike] += quantity
		} else if optionType == "P" {
			putStrikes[strike] += quantity
		}
	}

	// Find matching call/put pairs (simple approach - assumes same quantity)
	for callStrike, callQty := range callStrikes {
		for putStrike, putQty := range putStrikes {
			if callQty == putQty && callQty > 0 {
				strangles = append(strangles, orphanedStrangle{
					putStrike:  putStrike,
					callStrike: callStrike,
					expiration: expiration,
					quantity:   callQty,
					symbol:     "SPY",
				})
			}
		}
	}

	return strangles
}

func strangleMatches(position models.Position, strangle orphanedStrangle) bool {
	return math.Abs(position.PutStrike-strangle.putStrike) < 0.01 &&
		math.Abs(position.CallStrike-strangle.callStrike) < 0.01 &&
		position.Expiration.Format("2006-01-02") == strangle.expiration &&
		position.Quantity == strangle.quantity
}

// parseOptionSymbol parses an OPRA format option symbol to extract strike and type
// Format: TICKER[YYMMDD][C/P][STRIKE*1000 padded to 8 digits]
// Example: SPY240315C00610000 -> strike=610.00, type="C"
func parseOptionSymbol(symbol string) (float64, string, error) {
	if len(symbol) < 15 {
		return 0, "", fmt.Errorf("option symbol too short: %s", symbol)
	}

	// Find the 6-digit YYMMDD expiration date pattern
	expirationPos := -1
	for i := 0; i <= len(symbol)-6; i++ {
		// Check if we have 6 consecutive digits
		if isAllDigits(symbol[i:i+6]) {
			expirationPos = i
			break
		}
	}

	if expirationPos == -1 {
		return 0, "", fmt.Errorf("no 6-digit expiration date (YYMMDD) found in symbol: %s", symbol)
	}

	// The option type (C/P) should be immediately after the expiration date
	optionTypePos := expirationPos + 6
	if optionTypePos >= len(symbol) {
		return 0, "", fmt.Errorf("symbol too short after expiration date: %s", symbol)
	}

	optionType := string(symbol[optionTypePos])
	if optionType != "C" && optionType != "P" {
		return 0, "", fmt.Errorf("invalid option type '%s' at position %d, expected 'C' or 'P' in symbol: %s", optionType, optionTypePos, symbol)
	}

	// Extract strike price (8 digits after the option type)
	strikeStart := optionTypePos + 1
	strikeEnd := strikeStart + 8
	if strikeEnd > len(symbol) {
		return 0, "", fmt.Errorf("symbol too short for 8-digit strike extraction, need %d characters but only have %d: %s", strikeEnd, len(symbol), symbol)
	}

	strikeStr := symbol[strikeStart:strikeEnd]
	if !isAllDigits(strikeStr) {
		return 0, "", fmt.Errorf("invalid strike format, expected 8 digits but got '%s' in symbol: %s", strikeStr, symbol)
	}

	strikeInt, err := strconv.ParseInt(strikeStr, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("failed to parse strike '%s' in symbol %s: %w", strikeStr, symbol, err)
	}

	strike := float64(strikeInt) / 1000.0
	return strike, optionType, nil
}

// isAllDigits checks if a string contains only digits
func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}