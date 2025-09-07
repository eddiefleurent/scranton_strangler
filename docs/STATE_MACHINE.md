# Position State Machine

The SPY Short Strangle bot uses a simplified state machine to manage position lifecycle and enforce the "Football System" rules.

## States Overview

### Core States
- **`idle`** - No active position, ready for new opportunities
- **`submitted`** - Orders submitted, awaiting fill
- **`open`** - Position opened, ready for management
- **`closed`** - Position closed successfully
- **`error`** - Error state requiring manual intervention

### Football System Management States
- **`first_down`** - Normal theta decay monitoring (healthy position)
- **`second_down`** - Strike challenged (price within 5 points)
- **`third_down`** - Strike breached (considering adjustments)
- **`fourth_down`** - Critical decision point (final adjustment or exit)

### Action States  
- **`adjusting`** - Executing position adjustment (rolling strikes)
- **`rolling`** - "Punting" - rolling to new expiration

## State Transitions

```
idle → submitted → open → first_down → second_down → third_down → fourth_down
       ↓             ↑         ↑            ↑            ↑            ↓
     closed ← ─ ─ ─ ─ ┴ ─ ─ ─ ┴ ─ ─ ─ ─ ─ ─ ┴ ─ ─ ─ ─ ─ ─ ┴ ─ ─ → rolling
                                                    ↓
                              adjusting ← ─ ─ ─ ─ ─ ┘
                                 ↓
                              first_down (recovery)
```

## Football System Rules

### **First Down** - Normal Operations
- **Condition**: Position opened, strikes safe
- **Action**: Monitor theta decay, collect premium
- **Next**: Second Down if strike challenged

### **Second Down** - Strike Challenged  
- **Condition**: Price within 5 points of either strike
- **Action**: Elevated monitoring, prepare adjustments
- **Options**: 
  - Recovery → First Down (price moves away)
  - Progression → Third Down (strike breached)
  - Adjustment → `adjusting` state (roll untested side)

### **Third Down** - Strike Breached
- **Condition**: Price has breached a strike
- **Action**: Consider defensive adjustments
- **Options**:
  - Recovery → First Down (successful adjustment)
  - Progression → Fourth Down (adjustment failed)
  - Adjustment → `adjusting` state (execute strategy)

### **Fourth Down** - Critical Decision
- **Condition**: Multiple failed adjustments or critical situation  
- **Action**: Final attempt or prepare emergency exit
- **Options**:
  - Recovery → First Down (miracle recovery)
  - Exit → `closed` (take loss/small profit)
  - Punt → `rolling` state (roll to new expiration)

## Adjustment Limits

The state machine enforces strict limits to prevent over-management:

- **Maximum Adjustments**: 3 per position
- **Maximum Time Rolls**: 1 per position ("punt")

These limits prevent the "death by a thousand cuts" scenario where continuous adjustments erode profits.

## Usage in Code

```go
// Create position with state machine
pos := NewPosition("SPY-001", "SPY", 400.0, 420.0, expiration, 1)

// Transition through states
pos.TransitionState(StateOpen, "position_filled")
pos.TransitionState(StateFirstDown, "start_management")

// Check current state
if pos.IsInManagement() {
    phase := pos.GetManagementPhase() // Returns 1-4 for First-Fourth Down
    fmt.Printf("Currently in %s (Phase %d)", pos.GetStateDescription(), phase)
}

// Check adjustment capabilities  
if pos.CanAdjust() {
    pos.TransitionState(StateAdjusting, "execute_adjustment")
}

// Validate state consistency
if err := pos.ValidateState(); err != nil {
    log.Printf("State validation failed: %v", err)
}
```

## Benefits

1. **Rule Enforcement**: Prevents invalid operations and over-management
2. **Audit Trail**: Clear record of position progression  
3. **Error Prevention**: Catches programming bugs before they cost money
4. **Football Metaphor**: Easy to understand management phases
5. **Automatic Validation**: Ensures data consistency

## State Descriptions

Each state provides a human-readable description:

- **First Down**: "Normal monitoring - position healthy, collecting theta"
- **Second Down**: "Strike challenged - price within 5 points, elevated monitoring"  
- **Third Down**: "Strike breached - considering adjustments or defensive actions"
- **Fourth Down**: "Critical situation - final adjustment attempt or prepare to exit"

This makes logging and monitoring much clearer than simple status codes.