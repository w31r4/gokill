package why

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestBuildAncestryIncludesTarget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pid := os.Getpid()
	ancestry, err := buildAncestry(ctx, pid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ancestry) == 0 {
		t.Fatal("expected ancestry to be non-empty")
	}
	if ancestry[len(ancestry)-1].PID != pid {
		t.Fatalf("expected last pid %d, got %d", pid, ancestry[len(ancestry)-1].PID)
	}
}
