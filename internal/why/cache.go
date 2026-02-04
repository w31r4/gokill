package why

import (
	"context"
	"strings"
	"sync"
	"time"
)

// cacheKey uniquely identifies a process for caching purposes.
// Using PID + StartTime ensures we don't return stale data for reused PIDs.
type cacheKey struct {
	PID       int
	StartTime int64 // Unix timestamp in seconds
}

// cacheEntry holds a cached analysis result with expiration.
type cacheEntry struct {
	result    *AnalysisResult
	expiresAt time.Time
}

// cachedAnalyzer wraps an analyzer with caching functionality.
type cachedAnalyzer struct {
	mu       sync.RWMutex
	entries  map[cacheKey]*cacheEntry
	ttl      time.Duration
	maxSize  int
	analyzer Analyzer
}

// NewCachedAnalyzer creates a new analyzer with result caching.
func NewCachedAnalyzer(ttl time.Duration) *cachedAnalyzer {
	return &cachedAnalyzer{
		entries:  make(map[cacheKey]*cacheEntry),
		ttl:      ttl,
		maxSize:  100,
		analyzer: &baseAnalyzer{},
	}
}

// Analyze returns cached results if available, otherwise performs analysis.
func (c *cachedAnalyzer) Analyze(ctx context.Context, pid int) (*AnalysisResult, error) {
	// Try to get from cache first
	startTime := getProcessStartTime(pid)
	key := cacheKey{PID: pid, StartTime: startTime}
	now := time.Now()

	c.mu.RLock()
	if entry, ok := c.entries[key]; ok && now.Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.result, nil
	}
	c.mu.RUnlock()

	// Perform analysis
	result, err := c.analyzer.Analyze(ctx, pid)
	if err != nil {
		return result, err
	}

	// Cache the result
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cacheResult(key, result, now)

	return result, nil
}

func (c *cachedAnalyzer) AnalyzeWithOptions(ctx context.Context, pid int, opts AnalyzeOptions) (*AnalysisResult, error) {
	result, err := c.Analyze(ctx, pid)
	if result == nil || (!opts.CollectEnv && !opts.EnvWarnings) {
		return result, err
	}

	clone := cloneAnalysisResult(result)
	clone = applyEnvOptions(ctx, clone, pid, opts)
	return clone, err
}

func (c *cachedAnalyzer) cacheResult(key cacheKey, result *AnalysisResult, now time.Time) {
	if c.maxSize <= 0 {
		return
	}

	// Evict expired entries if cache is full
	if len(c.entries) >= c.maxSize {
		c.evictExpired(now)
	}

	for len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &cacheEntry{
		result:    result,
		expiresAt: now.Add(c.ttl),
	}
}

// evictExpired removes expired entries from the cache.
// Must be called with write lock held.
func (c *cachedAnalyzer) evictExpired(now time.Time) {
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

func (c *cachedAnalyzer) evictOldest() {
	var oldestKey cacheKey
	var oldest time.Time
	found := false

	for key, entry := range c.entries {
		if !found || entry.expiresAt.Before(oldest) {
			oldest = entry.expiresAt
			oldestKey = key
			found = true
		}
	}

	if found {
		delete(c.entries, oldestKey)
	}
}

// ClearCache removes all entries from the cache.
func (c *cachedAnalyzer) ClearCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[cacheKey]*cacheEntry)
}

// baseAnalyzer performs the actual analysis without caching.
type baseAnalyzer struct{}

// Analyze performs process ancestry analysis.
func (a *baseAnalyzer) Analyze(ctx context.Context, pid int) (*AnalysisResult, error) {
	result := &AnalysisResult{
		Source: Source{
			Type:       SourceUnknown,
			Confidence: 0.0,
		},
	}

	// Build ancestry chain
	ancestry, err := buildAncestry(ctx, pid)
	result.Ancestry = ancestry
	result.RestartCount = restartCountFromAncestry(ancestry)

	// Get working directory
	if len(ancestry) > 0 {
		result.WorkingDir = ancestry[len(ancestry)-1].WorkingDir
	}

	// Detect if the process is running from a deleted binary (best-effort, Linux-only).
	result.ExeDeleted = isProcessExeDeleted(pid)

	// Resolve systemd unit name (Linux-only, best-effort).
	if len(ancestry) > 0 {
		root := ancestry[0]
		if root.PID == 1 && strings.Contains(strings.ToLower(root.Command), "systemd") {
			result.SystemdUnit = resolveSystemdUnit(ctx, pid)
		}
	}

	// Detect source
	source := detectSource(ctx, ancestry)
	result.Source = source
	if source.Type == SourceDocker && source.Name != "" && source.Name != "container" {
		result.ContainerID = source.Name
	}

	// Detect Git context
	if result.WorkingDir != "" {
		result.GitRepo, result.GitBranch = detectGitInfo(result.WorkingDir)
	}

	// Perform health checks on the target process
	if len(ancestry) > 0 {
		targetProcess := &ancestry[len(ancestry)-1]
		result.Warnings = HealthCheck(targetProcess)
	}
	if shouldWarnRestart(result.RestartCount) {
		result.Warnings = append(result.Warnings, restartWarning(result.RestartCount))
	}
	result.Warnings = append(result.Warnings, commonWarnings(result)...)
	result.Warnings = dedupeStringsPreserveOrder(result.Warnings)

	if err != nil {
		// Return partial result even on error
		return result, err
	}

	return result, nil
}

// getProcessStartTime returns the start time of a process as Unix timestamp.
// Returns 0 if the process cannot be found.
func getProcessStartTime(pid int) int64 {
	// This is implemented in platform-specific files
	return getProcessStartTimePlatform(pid)
}
