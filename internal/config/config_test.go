package config

import (
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Test with example config file (should work for basic structure validation)
	configPath := filepath.Join("..", "..", "config.yaml.example")
	_, err := Load(configPath)
	if err != nil {
		t.Errorf("Expected config to load successfully from example file, got error: %v", err)
	}
}

func TestLoad_InvalidPath(t *testing.T) {
	_, err := Load("nonexistent.yaml")
	if err == nil {
		t.Error("Expected error when loading nonexistent config file, got nil")
	}
}
