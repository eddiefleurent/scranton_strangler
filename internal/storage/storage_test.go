package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func mustTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "storage_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func TestNewJSONStorage(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "test.json")
	
	storage, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage failed: %v", err)
	}
	
	if storage == nil {
		t.Fatal("Expected non-nil storage")
	}
	
	// Verify initial state
	positions := storage.GetCurrentPositions()
	if len(positions) != 0 {
		t.Errorf("Expected 0 initial positions, got %d", len(positions))
	}
}

// Additional tests would go here, focused on the new multi-position API
// The comprehensive interface tests in interface_test.go provide the main coverage