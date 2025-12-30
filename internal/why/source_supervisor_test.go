package why

import (
	"testing"
)

func TestDetectSupervisor(t *testing.T) {
	tests := []struct {
		name     string
		ancestry []ProcessInfo
		expected *Source
	}{
		{
			name: "PM2 Process",
			ancestry: []ProcessInfo{
				{Command: "node", Cmdline: "node /usr/local/bin/pm2 start app.js"},
			},
			expected: &Source{Type: SourcePM2, Name: "pm2"},
		},
		{
			name: "Supervisord Child",
			ancestry: []ProcessInfo{
				{Command: "python", Cmdline: "python app.py"},
				{Command: "supervisord", Cmdline: "/usr/bin/supervisord"},
			},
			expected: &Source{Type: SourceSupervisor, Name: "supervisord"},
		},
		{
			name: "Gunicorn Worker",
			ancestry: []ProcessInfo{
				{Command: "gunicorn", Cmdline: "gunicorn: worker [myproject]"},
			},
			expected: &Source{Type: SourceSupervisor, Name: "gunicorn"},
		},
		{
			name: "Unknown Process",
			ancestry: []ProcessInfo{
				{Command: "bash", Cmdline: "/bin/bash"},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectSupervisor(tt.ancestry)
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
