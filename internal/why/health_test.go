package why

import (
	"testing"
	"time"
)

func TestHealthCheck_StatusFirstChar(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{name: "ZombieSingleChar", status: "Z", expected: "Process is a zombie (defunct)"},
		{name: "ZombieWithFlags", status: "Z+", expected: "Process is a zombie (defunct)"},
		{name: "StoppedSingleChar", status: "T", expected: "Process is stopped"},
		{name: "StoppedWithFlags", status: "T+", expected: "Process is stopped"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := HealthCheck(&ProcessInfo{Status: tt.status})
			found := false
			for _, w := range warnings {
				if w == tt.expected {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected warning %q, got %v", tt.expected, warnings)
			}
		})
	}
}

func TestHealthCheck_RootAndLongRunningAndHighMemory(t *testing.T) {
	p := &ProcessInfo{
		User:      "root",
		StartedAt: time.Now().Add(-91 * 24 * time.Hour),
		RSS:       1024*1024*1024 + 1,
	}

	warnings := HealthCheck(p)

	expect := []string{
		"Process is running as root",
		"Process has been running for over 90 days",
	}

	for _, e := range expect {
		found := false
		for _, w := range warnings {
			if w == e {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected warning %q, got %v", e, warnings)
		}
	}

	foundHighMem := false
	for _, w := range warnings {
		if len(w) >= len("Process is using high memory") && w[:len("Process is using high memory")] == "Process is using high memory" {
			foundHighMem = true
			break
		}
	}
	if !foundHighMem {
		t.Fatalf("expected high memory warning, got %v", warnings)
	}
}

func TestHealthCheck_HighCPUTime(t *testing.T) {
	p := &ProcessInfo{
		CPUTime: 2*time.Hour + time.Second,
	}
	warnings := HealthCheck(p)

	found := false
	for _, w := range warnings {
		if w == "Process has high accumulated CPU time (>2h)" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected high CPU time warning, got %v", warnings)
	}
}
