package why

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectContainer(t *testing.T) {
	// Create a temporary directory to simulate /proc
	tmpDir, err := os.MkdirTemp("", "gkill-test-proc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create proc directory
	procDir := filepath.Join(tmpDir, "proc")
	if err := os.Mkdir(procDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		pid      int
		content  string
		expected *Source
	}{
		{
			name:    "Docker Container",
			pid:     1001,
			content: "12:pids:/docker/c6c6e7e7...\n1:name=systemd:/docker/c6c6e7e7...",
			expected: &Source{
				Type:       SourceDocker,
				Name:       "container",
				Confidence: 0.9,
			},
		},
		{
			name:    "Kubernetes Pod",
			pid:     1002,
			content: "12:pids:/kubepods/burstable/...\n1:name=systemd:/kubepods/...",
			expected: &Source{
				Type:       SourceDocker, // Mapped to Docker/Container type for now
				Name:       "container",
				Confidence: 0.9,
			},
		},
		{
			name:     "Non-Container Process",
			pid:      1003,
			content:  "12:pids:/user.slice/user-1000.slice/...\n1:name=systemd:/user.slice/...",
			expected: nil,
		},
		{
			name:     "Missing File",
			pid:      9999,
			content:  "", // File won't be created
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.content != "" {
				pidDir := filepath.Join(procDir, fmt.Sprintf("%d", tt.pid))
				if err := os.Mkdir(pidDir, 0755); err != nil {
					t.Fatal(err)
				}
				cgroupPath := filepath.Join(pidDir, "cgroup")
				if err := os.WriteFile(cgroupPath, []byte(tt.content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			ancestry := []ProcessInfo{
				{PID: tt.pid},
			}

			result := detectContainer(ancestry, tmpDir)

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
