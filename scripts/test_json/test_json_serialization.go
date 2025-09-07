package main

import (
	"encoding/json"
	"fmt"
	"github.com/eddiefleurent/scranton_strangler/internal/models"
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

	fmt.Printf("Original Position State: %s\n", pos.GetCurrentState())
	fmt.Printf("Original StateMachine is nil: %v\n", pos.StateMachine == nil)

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

	fmt.Printf("Deserialized Position State: %s\n", deserializedPos.GetCurrentState())
	fmt.Printf("Deserialized StateMachine is nil: %v\n", deserializedPos.StateMachine == nil)

	// Test that lazy initialization works
	fmt.Printf("Deserialized Management Phase: %d\n", deserializedPos.GetManagementPhase())
	fmt.Printf("After lazy init, StateMachine is nil: %v\n", deserializedPos.StateMachine == nil)
}
