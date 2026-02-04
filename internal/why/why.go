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
	Type       SourceType // The type of source (systemd, launchd, etc.)
	Name       string     // Service/unit name if available
	Confidence float64    // Confidence score 0.0-1.0
}

// AnalysisResult contains the complete analysis of why a process is running.
type AnalysisResult struct {
	Ancestry     []ProcessInfo // Process chain from init to target
	Source       Source        // Detected process source/supervisor
	WorkingDir   string        // Working directory of target process
	GitRepo      string        // Git repository name (if applicable)
	GitBranch    string        // Git branch (if applicable)
	SystemdUnit  string        // systemd unit name (best-effort, Linux-only)
	Env          []string      // Environment variables (key=value, best-effort)
	EnvError     string        // Environment read error (best-effort)
	ExeDeleted   bool          // True if executable is deleted (best-effort)
	RestartCount int           // Consecutive restart count (best-effort)
	ContainerID  string        // Container identifier (best-effort)
	Warnings     []string      // Health/security warnings
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

type AnalyzeOptions struct {
	// CollectEnv controls whether the full environment is included in the result.
	// Env values may contain secrets; callers should prefer redaction at render time.
	CollectEnv bool

	// EnvWarnings controls whether env-based suspicious indicators are checked and appended to warnings.
	// When enabled, the analyzer may read the process environment best-effort even if CollectEnv is false.
	EnvWarnings bool
}

// AnalyzeWithOptions is like Analyze, but allows optional collectors that should not be cached by default.
func AnalyzeWithOptions(ctx context.Context, pid int, opts AnalyzeOptions) (*AnalysisResult, error) {
	if a, ok := DefaultAnalyzer.(*cachedAnalyzer); ok {
		return a.AnalyzeWithOptions(ctx, pid, opts)
	}

	result, err := Analyze(ctx, pid)
	if result == nil || (!opts.CollectEnv && !opts.EnvWarnings) {
		return result, err
	}

	clone := cloneAnalysisResult(result)
	clone = applyEnvOptions(ctx, clone, pid, opts)
	return clone, err
}

// AnalyzeWithTimeout is a convenience function that adds a timeout to the analysis.
func AnalyzeWithTimeout(pid int, timeout time.Duration) (*AnalysisResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return AnalyzeWithOptions(ctx, pid, AnalyzeOptions{CollectEnv: true, EnvWarnings: true})
}

func AnalyzeWithTimeoutOptions(pid int, timeout time.Duration, opts AnalyzeOptions) (*AnalysisResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return AnalyzeWithOptions(ctx, pid, opts)
}

func cloneAnalysisResult(r *AnalysisResult) *AnalysisResult {
	if r == nil {
		return nil
	}

	clone := *r
	if len(r.Ancestry) > 0 {
		clone.Ancestry = append([]ProcessInfo(nil), r.Ancestry...)
	}
	if len(r.Env) > 0 {
		clone.Env = append([]string(nil), r.Env...)
	}
	if len(r.Warnings) > 0 {
		clone.Warnings = append([]string(nil), r.Warnings...)
	}
	return &clone
}

func applyEnvOptions(ctx context.Context, r *AnalysisResult, pid int, opts AnalyzeOptions) *AnalysisResult {
	if r == nil || pid <= 0 || (!opts.CollectEnv && !opts.EnvWarnings) {
		return r
	}

	if ctx == nil {
		ctx = context.Background()
	}
	envCtx, cancel := context.WithTimeout(ctx, defaultEnvReadTimeout)
	defer cancel()

	env, envErr := readProcessEnvWithContext(envCtx, pid, defaultEnvMaxBytes, defaultEnvMaxVars)
	if envErr != nil && opts.CollectEnv {
		r.EnvError = envErr.Error()
	}
	if len(env) == 0 {
		return r
	}

	if opts.EnvWarnings {
		r.Warnings = append(r.Warnings, envSuspiciousWarnings(env)...)
		r.Warnings = dedupeStringsPreserveOrder(r.Warnings)
	}

	if opts.CollectEnv {
		r.Env = env
	}

	return r
}
