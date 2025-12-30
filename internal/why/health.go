package why

import (
	"fmt"
	"time"
)

// HealthCheck checks the health of a process and returns a list of warnings.
//
// Note: Some Phase 2 checks (e.g. public listener, high CPU time) require
// additional input that is not currently part of ProcessInfo, so they are
// intentionally not implemented here yet.
func HealthCheck(p *ProcessInfo) []string {
	var warnings []string
	if p == nil {
		return warnings
	}

	// Process state checks (best-effort across platforms).
	// macOS `ps state` may include extra flags, so we match by first character.
	if len(p.Status) > 0 {
		switch p.Status[0] {
		case 'Z':
			warnings = append(warnings, "Process is a zombie (defunct)")
		case 'T':
			warnings = append(warnings, "Process is stopped")
		}
	}

	// Root execution check (best-effort).
	if p.User == "root" {
		warnings = append(warnings, "Process is running as root")
	}

	// Long-running process check (> 90 days).
	if !p.StartedAt.IsZero() {
		const longRunning = 90 * 24 * time.Hour
		if time.Since(p.StartedAt) > longRunning {
			warnings = append(warnings, "Process has been running for over 90 days")
		}
	}

	// High memory usage (RSS > 1 GiB).
	const highMemThreshold = 1024 * 1024 * 1024
	if p.RSS > highMemThreshold {
		rssMB := float64(p.RSS) / (1024 * 1024)
		warnings = append(warnings, fmt.Sprintf("Process is using high memory (%.2f MB RSS)", rssMB))
	}

	return warnings
}
