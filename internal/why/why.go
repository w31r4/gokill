// Package why provides process ancestry analysis and source detection.
// It answers the question "Why is this process running?" by building
// a causal chain from init/systemd to the target process.
package why

import (
	"context"
	"time"
)

// ProcessInfo contains information about a single process in the ancestry chain.
type ProcessInfo struct {
	PID        int           // Process ID
	PPID       int           // Parent Process ID
	Command    string        // Short command name (e.g., "node")
	Cmdline    string        // Full command line
	User       string        // Username running the process
	StartedAt  time.Time     // Process start time
	WorkingDir string        // Current working directory
	Status     string        // Process status (R, S, Z, etc.)
	RSS        uint64        // Resident Set Size (bytes)
	CPUTime    time.Duration // Total CPU time consumed (best-effort)
}

// SourceType represents the type of process supervisor or launcher.
type SourceType string

const (
	SourceSystemd    SourceType = "systemd"
	SourceLaunchd    SourceType = "launchd"
	SourceDocker     SourceType = "docker"
	SourcePM2        SourceType = "pm2"
	SourceSupervisor SourceType = "supervisor"
	SourceCron       SourceType = "cron"
	SourceShell      SourceType = "shell"
	SourceUnknown    SourceType = "unknown"
)

// Source represents the detected origin/supervisor of a process.
type Source struct {
	Type       SourceType        // The type of source (systemd, launchd, etc.)
	Name       string            // Service/unit name if available
	Confidence float64           // Confidence score 0.0-1.0
	Details    map[string]string // Additional metadata
}

// AnalysisResult contains the complete analysis of why a process is running.
type AnalysisResult struct {
	Ancestry   []ProcessInfo // Process chain from init to target
	Source     Source        // Detected process source/supervisor
	WorkingDir string        // Working directory of target process
	GitRepo    string        // Git repository name (if applicable)
	GitBranch  string        // Git branch (if applicable)
	Warnings   []string      // Health/security warnings
}

// Analyzer provides process ancestry analysis.
type Analyzer interface {
	// Analyze returns ancestry and source information for the given PID.
	Analyze(ctx context.Context, pid int) (*AnalysisResult, error)
}

// DefaultAnalyzer is the default implementation of Analyzer with caching.
var DefaultAnalyzer Analyzer

func init() {
	DefaultAnalyzer = NewCachedAnalyzer(5 * time.Minute)
}

// Analyze is a convenience function that uses the DefaultAnalyzer.
func Analyze(ctx context.Context, pid int) (*AnalysisResult, error) {
	return DefaultAnalyzer.Analyze(ctx, pid)
}

// AnalyzeWithTimeout is a convenience function that adds a timeout to the analysis.
func AnalyzeWithTimeout(pid int, timeout time.Duration) (*AnalysisResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return Analyze(ctx, pid)
}
