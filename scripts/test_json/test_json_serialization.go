package main

import (
	"encoding/json"
	"fmt"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
	"log"
	"reflect"
	"strings"
	"time"
)

func main() {
	// Create a new position
	pos := models.NewPosition("test-pos", "SPY", 400, 410, time.Now().AddDate(0, 0, 45), 1)

	// Transition to a different state to test persistence
	err := pos.TransitionState(models.StateSubmitted, "order_placed")
	if err != nil {
		fmt.Printf("Error transitioning: %v\n", err)
		return
	}
	err = pos.TransitionState(models.StateOpen, "order_filled")
	if err != nil {
		fmt.Printf("Error transitioning: %v\n", err)
		return
	}

	originalState := pos.GetCurrentState()
	originalSMIsNil := pos.StateMachine == nil
	fmt.Printf("Original Position State: %s\n", originalState)
	fmt.Printf("Original StateMachine is nil: %v\n", originalSMIsNil)

	// Serialize to JSON
	jsonData, err := json.MarshalIndent(pos, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling: %v\n", err)
		return
	}

	fmt.Printf("JSON representation:\n%s\n", string(jsonData))

	// Validate JSON and ensure StateMachine is omitted
	if !json.Valid(jsonData) {
		fmt.Println("JSON is not valid!")
		return
	}
	if strings.Contains(string(jsonData), "StateMachine") {
		fmt.Println("StateMachine leaked into JSON!")
		return
	}

	// Deserialize from JSON
	var deserializedPos models.Position
	err = json.Unmarshal(jsonData, &deserializedPos)
	if err != nil {
		fmt.Printf("Error unmarshaling: %v\n", err)
		return
	}

	// Assert that deserialized state equals original state
	deserializedState := deserializedPos.GetCurrentState()
	if deserializedState != originalState {
		log.Fatalf("State mismatch after deserialization: expected %s, got %s", originalState, deserializedState)
	}

	// Assert StateMachine is nil after deserialization
	deserializedSMIsNil := deserializedPos.StateMachine == nil
	if !deserializedSMIsNil {
		log.Fatalf("StateMachine should be nil after deserialization, but is not nil")
	}

	// Test that lazy initialization works
	originalManagementPhase := pos.GetManagementPhase()
	deserializedManagementPhase := deserializedPos.GetManagementPhase()
	if deserializedManagementPhase != originalManagementPhase {
		log.Fatalf("Management phase mismatch: expected %d, got %d", originalManagementPhase, deserializedManagementPhase)
	}

	// Assert StateMachine is no longer nil after lazy initialization
	afterLazyInitSMIsNil := deserializedPos.StateMachine == nil
	if afterLazyInitSMIsNil {
		log.Fatalf("StateMachine should not be nil after lazy initialization, but is nil")
	}

	// Assert that the full deserialized position equals the original (excluding StateMachine)
	if !reflect.DeepEqual(pos.ID, deserializedPos.ID) ||
		!reflect.DeepEqual(pos.Symbol, deserializedPos.Symbol) ||
		!reflect.DeepEqual(pos.PutStrike, deserializedPos.PutStrike) ||
		!reflect.DeepEqual(pos.CallStrike, deserializedPos.CallStrike) ||
		!reflect.DeepEqual(pos.Expiration, deserializedPos.Expiration) ||
		!reflect.DeepEqual(pos.Quantity, deserializedPos.Quantity) {
		log.Fatalf("Deserialized position data does not match original")
	}

	fmt.Printf("✅ All assertions passed: JSON serialization/deserialization working correctly")
}
