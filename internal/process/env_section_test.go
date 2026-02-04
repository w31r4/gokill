package process

import (
	"strings"
	"testing"

	"github.com/w31r4/gokill/internal/why"
)

func TestAppendEnvSectionUnavailable(t *testing.T) {
	var b strings.Builder
	appendEnvSection(&b, &why.AnalysisResult{EnvError: "permission denied"}, DetailsOptions{ShowEnv: true})
	out := b.String()
	if !strings.Contains(out, "Env:") {
		t.Fatalf("expected Env header, got: %q", out)
	}
	if !strings.Contains(out, "unavailable") {
		t.Fatalf("expected unavailable message, got: %q", out)
	}
}

func TestAppendEnvSectionPartial(t *testing.T) {
	var b strings.Builder
	appendEnvSection(&b, &why.AnalysisResult{Env: []string{"A=1"}, EnvError: "truncated: maxBytes=65536"}, DetailsOptions{ShowEnv: true})
	out := b.String()
	if !strings.Contains(out, "Env:") {
		t.Fatalf("expected Env header, got: %q", out)
	}
	if !strings.Contains(out, "(partial:") {
		t.Fatalf("expected partial message, got: %q", out)
	}
}
