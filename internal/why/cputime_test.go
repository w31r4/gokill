package why

import (
	"testing"
	"time"
)

func TestParsePsCPUTime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"01:02", 1*time.Minute + 2*time.Second},
		{"1:02:03", 1*time.Hour + 2*time.Minute + 3*time.Second},
		{"2-03:04:05", 2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second},
		{"", 0},
		{"bad", 0},
	}

	for _, tt := range tests {
		if got := parsePsCPUTime(tt.input); got != tt.expected {
			t.Fatalf("parsePsCPUTime(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}
