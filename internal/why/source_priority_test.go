package why

import "testing"

func TestPickBestSourcePrefersStablePriorityOverConfidence(t *testing.T) {
	candidates := []*Source{
		{Type: SourceShell, Name: "zsh", Confidence: 0.99},
		{Type: SourceSystemd, Name: "foo.service", Confidence: 0.2},
		{Type: SourceCron, Name: "cron", Confidence: 0.95},
		{Type: SourceSupervisor, Name: "supervisord", Confidence: 0.3},
	}

	best := pickBestSource(candidates)
	if best == nil {
		t.Fatalf("expected best source, got nil")
	}

	// Per priority order in pickBestSource(): Supervisor/PM2 beats Cron, Systemd, Shell.
	if best.Type != SourceSupervisor {
		t.Fatalf("expected best.Type=%s, got %s", SourceSupervisor, best.Type)
	}
	if best.Name != "supervisord" {
		t.Fatalf("expected best.Name=supervisord, got %q", best.Name)
	}
}

func TestPickBestSourceTiesBreakByConfidenceWithinSamePriority(t *testing.T) {
	candidates := []*Source{
		{Type: SourceSupervisor, Name: "supervisord", Confidence: 0.4},
		{Type: SourcePM2, Name: "pm2", Confidence: 0.7},
	}

	best := pickBestSource(candidates)
	if best == nil {
		t.Fatalf("expected best source, got nil")
	}

	// Supervisor and PM2 share the same priority bucket; higher confidence should win.
	if best.Type != SourcePM2 {
		t.Fatalf("expected best.Type=%s, got %s", SourcePM2, best.Type)
	}
}

func TestPickBestSourceHandlesNilAndUnknownTypes(t *testing.T) {
	candidates := []*Source{
		nil,
		{Type: SourceType("some-new-type"), Name: "x", Confidence: 0.9},
		{Type: SourceUnknown, Name: "", Confidence: 0.1},
	}

	best := pickBestSource(candidates)
	if best == nil {
		t.Fatalf("expected best source, got nil")
	}

	// Unknown/unrecognized types should be treated as SourceUnknown priority.
	// In that case, confidence tie-break should pick the higher confidence entry.
	if best.Type != SourceType("some-new-type") {
		t.Fatalf("expected best.Type=some-new-type, got %s", best.Type)
	}
	if best.Name != "x" {
		t.Fatalf("expected best.Name=x, got %q", best.Name)
	}
}
