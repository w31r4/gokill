package why

import (
	"testing"
)

func TestDetectCron(t *testing.T) {
	tests := []struct {
		name     string
		ancestry []ProcessInfo
		expected *Source
	}{
		{
			name: "Cron Process",
			ancestry: []ProcessInfo{
				{Command: "cron"},
			},
			expected: &Source{Type: SourceCron, Name: "cron"},
		},
		{
			name: "Crond Process",
			ancestry: []ProcessInfo{
				{Command: "crond"},
			},
			expected: &Source{Type: SourceCron, Name: "cron"},
		},
		{
			name: "Cron Child",
			ancestry: []ProcessInfo{
				{Command: "sh"},
				{Command: "cron"},
			},
			expected: &Source{Type: SourceCron, Name: "cron"},
		},
		{
			name: "Non-Cron Process",
			ancestry: []ProcessInfo{
				{Command: "bash"},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectCron(tt.ancestry)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("expected %v, got nil", tt.expected)
				} else if result.Type != tt.expected.Type || result.Name != tt.expected.Name {
					t.Errorf("expected %v, got %v", tt.expected, result)
				}
			}
		})
	}
}
