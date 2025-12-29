package why

import (
	"context"
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
	analyzer *baseAnalyzer
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

	c.mu.RLock()
	if entry, ok := c.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.result, nil
	}
	c.mu.RUnlock()

	// Perform analysis
	result, err := c.analyzer.Analyze(ctx, pid)
	if err != nil {
		return nil, err
	}

	// Cache the result
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict expired entries if cache is full
	if len(c.entries) >= c.maxSize {
		c.evictExpired()
	}

	c.entries[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}

	return result, nil
}

// evictExpired removes expired entries from the cache.
// Must be called with write lock held.
func (c *cachedAnalyzer) evictExpired() {
	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
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
	if err != nil {
		// Return partial result even on error
		return result, nil
	}
	result.Ancestry = ancestry

	// Get working directory
	if len(ancestry) > 0 {
		result.WorkingDir = ancestry[len(ancestry)-1].WorkingDir
	}

	// Detect source
	source := detectSource(ctx, ancestry)
	result.Source = source

	return result, nil
}

// getProcessStartTime returns the start time of a process as Unix timestamp.
// Returns 0 if the process cannot be found.
func getProcessStartTime(pid int) int64 {
	// This is implemented in platform-specific files
	return getProcessStartTimePlatform(pid)
}
