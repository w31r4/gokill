package why

import (
	"strings"
	"testing"
)

func TestEnvSuspiciousWarnings(t *testing.T) {
	t.Run("LD_PRELOAD", func(t *testing.T) {
		w := envSuspiciousWarnings([]string{"LD_PRELOAD=/tmp/x.so"})
		if len(w) != 1 || !strings.Contains(w[0], "LD_PRELOAD") {
			t.Fatalf("unexpected warnings: %#v", w)
		}
	})

	t.Run("DYLDKeys", func(t *testing.T) {
		w := envSuspiciousWarnings([]string{
			"DYLD_LIBRARY_PATH=/tmp",
			"DYLD_INSERT_LIBRARIES=/tmp/x.dylib",
		})
		if len(w) != 1 || !strings.Contains(w[0], "DYLD_") {
			t.Fatalf("unexpected warnings: %#v", w)
		}
		if !strings.Contains(w[0], "DYLD_INSERT_LIBRARIES") || !strings.Contains(w[0], "DYLD_LIBRARY_PATH") {
			t.Fatalf("expected DYLD keys to be included: %#v", w)
		}
	})
}

func TestCommonWarnings(t *testing.T) {
	r := &AnalysisResult{
		Source:     Source{Type: SourceUnknown},
		WorkingDir: "/tmp",
		ExeDeleted: true,
	}

	w := commonWarnings(r)
	joined := strings.Join(w, "\n")
	if !strings.Contains(joined, "deleted binary") {
		t.Fatalf("expected deleted-binary warning, got: %#v", w)
	}
	if !strings.Contains(joined, "No known supervisor") {
		t.Fatalf("expected unknown-supervisor warning, got: %#v", w)
	}
	if !strings.Contains(joined, "suspicious working directory") {
		t.Fatalf("expected working-dir warning, got: %#v", w)
	}
}
