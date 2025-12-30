package why

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubAnalyzer struct {
	result *AnalysisResult
	err    error
}

func (s stubAnalyzer) Analyze(ctx context.Context, pid int) (*AnalysisResult, error) {
	return s.result, s.err
}

func TestCachedAnalyzerReturnsPartialOnError(t *testing.T) {
	expected := &AnalysisResult{
		Source: Source{Type: SourceUnknown},
	}
	expectedErr := errors.New("boom")

	c := &cachedAnalyzer{
		entries:  make(map[cacheKey]*cacheEntry),
		ttl:      time.Minute,
		maxSize:  10,
		analyzer: stubAnalyzer{result: expected, err: expectedErr},
	}

	got, err := c.Analyze(context.Background(), 123456)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if got != expected {
		t.Fatalf("expected result %v, got %v", expected, got)
	}
	if len(c.entries) != 0 {
		t.Fatalf("expected no cache entries, got %d", len(c.entries))
	}
}

func TestCachedAnalyzerEvictsOldestWhenFull(t *testing.T) {
	now := time.Now()
	key1 := cacheKey{PID: 111111, StartTime: 0}
	key2 := cacheKey{PID: 222222, StartTime: 0}

	c := &cachedAnalyzer{
		entries: map[cacheKey]*cacheEntry{
			key1: {result: &AnalysisResult{}, expiresAt: now.Add(10 * time.Minute)},
			key2: {result: &AnalysisResult{}, expiresAt: now.Add(20 * time.Minute)},
		},
		ttl:      time.Minute,
		maxSize:  2,
		analyzer: stubAnalyzer{result: &AnalysisResult{}, err: nil},
	}

	pid3 := 333333
	if _, err := c.Analyze(context.Background(), pid3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.entries) != c.maxSize {
		t.Fatalf("expected cache size %d, got %d", c.maxSize, len(c.entries))
	}

	if _, ok := c.entries[key1]; ok {
		t.Fatalf("expected oldest entry to be evicted")
	}

	key3 := cacheKey{PID: pid3, StartTime: getProcessStartTime(pid3)}
	if _, ok := c.entries[key3]; !ok {
		t.Fatalf("expected new entry to be cached")
	}
}
