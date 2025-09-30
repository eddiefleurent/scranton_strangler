package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/broker"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"github.com/eddiefleurent/scranton_strangler/internal/storage"
	"github.com/google/uuid"
)

// Reconciler handles position synchronization between broker and storage
type Reconciler struct {
	broker         broker.Broker
	storage        storage.Interface
	logger         *log.Logger
	coldStartOnce  sync.Once
	phantomThreshold time.Duration
}

// NewReconciler creates a new position reconciler
func NewReconciler(broker broker.Broker, storage storage.Interface, logger *log.Logger, phantomThreshold time.Duration) *Reconciler {
	return &Reconciler{
		broker:  broker,
		storage: storage,
		logger:  logger,
		phantomThreshold: phantomThreshold,
	}
}

const positionsFetchTimeout = 8 * time.Second

// ReconcilePositions detects position mismatches between broker and storage
// It handles three cases:
// 1. Positions in storage but closed in broker (manual closes)
// 2. Positions in broker but missing from storage (timeout-related sync issues)
// 3. Cold start: stored positions are empty while broker has positions.
//    We log this and rely on the orphan-detection pass below to create recovery positions.
func (r *Reconciler) ReconcilePositions(storedPositions []models.Position) []models.Position {
	// Get current broker positions with timeout to prevent stuck cycles
	ctx, cancel := context.WithTimeout(context.Background(), positionsFetchTimeout)
	defer cancel()
	brokerPositions, err := r.broker.GetPositionsCtx(ctx)
	if err != nil {
		r.logger.Printf("Failed to get broker positions for reconciliation: %v", err)
		return storedPositions // Return unchanged on error
	}

	r.logger.Printf("Reconciling %d stored positions with %d broker positions",
		len(storedPositions), len(brokerPositions))

	// CRITICAL: Cold start scenario - if no stored positions but broker has positions,
	// this is likely a production restart without positions.json file
	if len(storedPositions) == 0 && len(brokerPositions) > 0 {
		// Use sync.Once to log cold start detection only once to avoid log spam
		r.coldStartOnce.Do(func() {
			r.logger.Printf("COLD START DETECTED: No stored positions but %d broker positions exist", len(brokerPositions))
			r.logger.Printf("Cold start: will attempt recovery via orphan-detection pass...")
		})
	}

	var activePositions []models.Position

	// First pass: Check stored positions against broker
	for _, position := range storedPositions {
		// Skip already closed positions
		if position.GetCurrentState() == models.StateClosed {
			continue
		}

		// PHANTOM POSITION DETECTION: Clean up positions with quantity=0 that never filled
		// These are created when orders timeout during polling but never actually executed
		// Check BEFORE updating LastChecked so we can use the old value for age calculation
		if position.Quantity == 0 && position.CreditReceived == 0 {
			// Determine age of position using LastChecked from previous reconciliation or EntryDate
			var timeSinceCreation time.Duration
			const stalePhantomThreshold = 24 * time.Hour // 24 hours for stale phantoms

			if !position.EntryDate.IsZero() {
				// If EntryDate is set, use it
				timeSinceCreation = time.Since(position.EntryDate)
			} else if !position.LastChecked.IsZero() {
				// Use LastChecked from previous reconciliation pass
				timeSinceCreation = time.Since(position.LastChecked)
			} else {
				// Never checked before, check if it has adjustment history indicating age
				if len(position.Adjustments) > 0 {
					// Has adjustment history, likely been around a while
					// Treat as stale (2x the normal threshold)
					timeSinceCreation = r.phantomThreshold * 2
				} else {
					// Brand new position with no history - wait for phantom threshold
					timeSinceCreation = 0
				}
			}

			// Use shorter threshold for recent phantoms, longer for stale ones
			threshold := r.phantomThreshold
			if timeSinceCreation > stalePhantomThreshold {
				// After 24 hours, definitely clean it up
				r.logger.Printf("STALE PHANTOM POSITION DETECTED: Position %s has quantity=0 and credit=0 after %.1f hours - cleaning up",
					shortID(position.ID), timeSinceCreation.Hours())

				if err := r.storage.DeletePosition(position.ID); err != nil {
					r.logger.Printf("Failed to clean up stale phantom position %s: %v", shortID(position.ID), err)
					activePositions = append(activePositions, position)
				} else {
					r.logger.Printf("Successfully cleaned up stale phantom position %s", shortID(position.ID))
				}
				continue
			} else if timeSinceCreation > threshold {
				r.logger.Printf("PHANTOM POSITION DETECTED: Position %s has quantity=0 and credit=0 after %.0f minutes - cleaning up",
					shortID(position.ID), timeSinceCreation.Minutes())

				// Delete this phantom position directly without state machine transitions
				// Phantoms never properly entered the system, so we just remove them
				if err := r.storage.DeletePosition(position.ID); err != nil {
					r.logger.Printf("Failed to clean up phantom position %s: %v", shortID(position.ID), err)
					activePositions = append(activePositions, position) // Keep in list if can't delete
				} else {
					r.logger.Printf("Successfully cleaned up phantom position %s", shortID(position.ID))
				}
				continue // Skip to next position
			}
		}

		// Update LastChecked timestamp after phantom detection
		position.LastChecked = time.Now().UTC()

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
			// Update LastChecked in storage
			if err := r.storage.UpdatePosition(&position); err != nil {
				r.logger.Printf("Warning: Failed to update LastChecked for position %s: %v", shortID(position.ID), err)
			}
			activePositions = append(activePositions, position)
		}
	}

	// Second pass: Check for orphaned broker positions (positions in broker but not in storage)
	// This handles the case where orders timed out locally but actually filled
	orphanedStrangles := r.findOrphanedStrangles(brokerPositions, activePositions)
	for _, orphanStrangle := range orphanedStrangles {
		r.logger.Printf("Detected orphaned strangle in broker: Put %.0f / Call %.0f",
			orphanStrangle.putStrike, orphanStrangle.callStrike)

		// First, check if we have a phantom position that matches this strangle
		// Phantoms are positions with quantity=0 that never filled, but now we see them in broker
		phantomToUpdate := r.findMatchingPhantom(activePositions, orphanStrangle)

		if phantomToUpdate != nil {
			// Update the existing phantom with broker data
			r.logger.Printf("Found matching phantom position %s, updating with broker data", shortID(phantomToUpdate.ID))
			phantomToUpdate.Quantity = orphanStrangle.quantity
			phantomToUpdate.CreditReceived = float64(-orphanStrangle.putCostBasis - orphanStrangle.callCostBasis)

			// Transition from submitted/idle to open state
			if err := phantomToUpdate.TransitionState(models.StateOpen, models.ConditionOrderFilled); err != nil {
				r.logger.Printf("Failed to transition phantom to open: %v", err)
			}

			if err := r.storage.UpdatePosition(phantomToUpdate); err != nil {
				r.logger.Printf("Failed to update phantom position: %v", err)
			} else {
				r.logger.Printf("Successfully updated phantom position %s with broker data", shortID(phantomToUpdate.ID))
			}
		} else {
			// No phantom found, create a recovery position
			r.logger.Printf("No matching phantom found, creating recovery position")
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
	}

	return activePositions
}

// orphanedStrangle represents a strangle position found in broker but not in storage
type orphanedStrangle struct {
	putStrike    float64
	callStrike   float64
	expiration   string
	quantity     int
	symbol       string
	putCostBasis float64  // Cost basis for put leg from broker
	callCostBasis float64 // Cost basis for call leg from broker
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
			trackedQty := 0
			for _, ap := range activePositions {
				if math.Abs(ap.PutStrike-strangle.putStrike) < 0.01 &&
					math.Abs(ap.CallStrike-strangle.callStrike) < 0.01 &&
					ap.Expiration.Format("2006-01-02") == strangle.expiration &&
					ap.Symbol == strangle.symbol {
					// Use absolute value to correctly count inventory regardless of sign
					if ap.Quantity < 0 {
						trackedQty += -ap.Quantity
					} else {
						trackedQty += ap.Quantity
					}
				}
			}
			missing := strangle.quantity - trackedQty
			if missing > 0 {
				s := strangle
				s.quantity = missing
				orphaned = append(orphaned, s)
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
	// Require broker to have at least the stored quantity for both legs (by strike)
	expectedQty := position.Quantity
	if expectedQty < 0 {
		expectedQty = -expectedQty
	}
	callQtyNet := 0
	putQtyNet := 0

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

		// Count contracts per matching strike/type (use signed quantities for net calculation)
		q := int(math.Round(brokerPos.Quantity))
		if optionType == "C" && math.Abs(parsedStrike-position.CallStrike) < 0.01 {
			callQtyNet += q
		} else if optionType == "P" && math.Abs(parsedStrike-position.PutStrike) < 0.01 {
			putQtyNet += q
		}
	}

	// Open only if broker holds at least the stored quantity for both legs (use abs of net to handle signed quantities)
	return absInt(callQtyNet) >= expectedQty && absInt(putQtyNet) >= expectedQty
}

// Helper functions

// absInt returns the absolute value of an integer
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

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
		allDigits := isAllDigits(symbol[i : i+6])
		
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
		if isAllDigits(symbol[i : i+6]) {
			// Validate C/P after the date for OPRA format confirmation
			if i+6 < len(symbol) {
				t := symbol[i+6]
				if t != 'C' && t != 'P' {
					continue
				}
			}
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

	callStrikes := make(map[float64]int)
	putStrikes := make(map[float64]int)
	callCostBasis := make(map[float64]float64) // Track cost basis for each strike
	putCostBasis := make(map[float64]float64)

	underlying := ""

	for _, pos := range positions {
		strike, optionType, err := parseOptionSymbol(pos.Symbol)
		if err != nil {
			continue
		}
		if underlying == "" {
			underlying = extractUnderlyingFromSymbol(pos.Symbol)
		}
		qty := int(math.Abs(pos.Quantity))
		if qty <= 0 {
			continue
		}
		if optionType == "C" {
			callStrikes[strike] += qty
			// Accumulate cost basis (negative values indicate credit received)
			callCostBasis[strike] += pos.CostBasis
		} else if optionType == "P" {
			putStrikes[strike] += qty
			putCostBasis[strike] += pos.CostBasis
		}
	}

	// deterministic ordering
	callKeys := make([]float64, 0, len(callStrikes))
	for k := range callStrikes {
		callKeys = append(callKeys, k)
	}
	putKeys := make([]float64, 0, len(putStrikes))
	for k := range putStrikes {
		putKeys = append(putKeys, k)
	}
	sort.Float64s(callKeys)
	sort.Float64s(putKeys)

	// greedy 1:1 pairing without reuse
	for _, ck := range callKeys {
		cRem := callStrikes[ck]
		if cRem == 0 {
			continue
		}
		for pi := 0; pi < len(putKeys) && cRem > 0; pi++ {
			pk := putKeys[pi]
			pRem := putStrikes[pk]
			if pRem == 0 {
				continue
			}
			n := cRem
			if pRem < n {
				n = pRem
			}
			// Scale average per-contract basis by n
			callAvg := 0.0
			if callStrikes[ck] > 0 {
				callAvg = callCostBasis[ck] / float64(callStrikes[ck])
			}
			putAvg := 0.0
			if putStrikes[pk] > 0 {
				putAvg = putCostBasis[pk] / float64(putStrikes[pk])
			}
			strangles = append(strangles, orphanedStrangle{
				putStrike:     pk,
				callStrike:    ck,
				expiration:    expiration,
				quantity:      n,
				putCostBasis:  putAvg * float64(n),
				callCostBasis: callAvg * float64(n),
				symbol:        underlying,
			})
			cRem -= n
			pRem -= n
			putStrikes[pk] = pRem
		}
	}

	return strangles
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
// findMatchingPhantom looks for a phantom position (quantity=0) that matches the orphaned strangle
func (r *Reconciler) findMatchingPhantom(positions []models.Position, orphan orphanedStrangle) *models.Position {
	for i := range positions {
		pos := &positions[i]

		// Check if this is a phantom (quantity = 0, credit = 0)
		if pos.Quantity != 0 || pos.CreditReceived != 0 {
			continue
		}

		// Check if strikes and expiration match
		expMatch := pos.Expiration.Format("2006-01-02") == orphan.expiration
		strikeMatch := math.Abs(pos.PutStrike-orphan.putStrike) < 0.01 &&
			math.Abs(pos.CallStrike-orphan.callStrike) < 0.01
		symbolMatch := pos.Symbol == orphan.symbol

		if expMatch && strikeMatch && symbolMatch {
			return pos
		}
	}

	return nil
}
