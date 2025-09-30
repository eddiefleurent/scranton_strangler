package main

import (
	"testing"
)

func TestMaskAccountID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "typical account ID",
			input:    "1234567890",
			expected: "******7890",
		},
		{
			name:     "short account ID (4 chars)",
			input:    "1234",
			expected: "1234",
		},
		{
			name:     "shorter than 4 chars",
			input:    "123",
			expected: "123",
		},
		{
			name:     "exactly 5 chars",
			input:    "12345",
			expected: "*2345",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single character",
			input:    "a",
			expected: "a",
		},
		{
			name:     "long account ID",
			input:    "1234567890123456",
			expected: "************3456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskAccountID(tt.input)
			if result != tt.expected {
				t.Errorf("maskAccountID(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}