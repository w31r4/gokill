package tui

import (
	"regexp"
	"strings"
	"testing"
)

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiEscapeRE.ReplaceAllString(s, "")
}

func TestComputeDetailLabelWidthMinAndCap(t *testing.T) {
	lines := []string{
		"  PID:\t123",
		"  Name:\tfoo",
	}
	if w := computeDetailLabelWidth(lines, 80); w < 12 {
		t.Fatalf("label width should be at least 12, got %d", w)
	}

	longLabelLines := []string{
		"  ExtremelyLongLabelNameThatShouldBeCapped:\tvalue",
	}
	if w := computeDetailLabelWidth(longLabelLines, 80); w > 24 {
		t.Fatalf("label width should be capped at 24, got %d", w)
	}
}

func TestFormatProcessDetailsWrapValueAndIndent(t *testing.T) {
	details := strings.Join([]string{
		"  PID:\t123",
		"  Command:\tthis is a very long command line with many words to wrap nicely",
	}, "\n") + "\n"

	out := stripANSI(formatProcessDetails(details, 40))
	lines := strings.Split(out, "\n")

	var cmdLineIdx int = -1
	for i, ln := range lines {
		if strings.Contains(ln, "Command:") {
			cmdLineIdx = i
			break
		}
	}
	if cmdLineIdx == -1 {
		t.Fatalf("expected output to include Command line, got:\n%s", out)
	}
	if cmdLineIdx+1 >= len(lines) {
		t.Fatalf("expected wrapped continuation line after Command, got:\n%s", out)
	}

	first := lines[cmdLineIdx]
	second := lines[cmdLineIdx+1]

	if !strings.Contains(first, "this is a very long") {
		t.Fatalf("expected first Command line to contain value prefix, got: %q", first)
	}
	if strings.Contains(second, "Command:") {
		t.Fatalf("expected continuation line not to repeat label, got: %q", second)
	}
	if strings.TrimLeft(second, " ") == second {
		t.Fatalf("expected continuation line to be indented, got: %q", second)
	}
	// Continuation content depends on wrap width; just ensure it contains later words from the value.
	if !(strings.Contains(second, "line with") || strings.Contains(second, "many words")) {
		t.Fatalf("expected continuation line to contain remaining words, got: %q", second)
	}
}

func TestFormatProcessDetailsWhySegments(t *testing.T) {
	details := strings.Join([]string{
		"  PID:\t123",
		"",
		"  ─────────────────────────────────────",
		"  Why It Exists:",
		"    systemd (pid 1) → init-systemd (pid 2) → SessionLeader (pid 1995)",
		"",
		"  Source:\tsystemd",
	}, "\n") + "\n"

	out := stripANSI(formatProcessDetails(details, 60))
	lines := strings.Split(out, "\n")

	var whyIdx int = -1
	for i, ln := range lines {
		if strings.Contains(ln, "Why It Exists:") {
			whyIdx = i
			break
		}
	}
	if whyIdx == -1 {
		t.Fatalf("expected output to include Why It Exists section, got:\n%s", out)
	}

	// Expect wrapping only happens on segment boundaries (between tokens), not within a token.
	// We do not assert exactly how many tokens appear per line, only that continuation lines start with "→ "
	// and tokens are not split apart.
	foundFirstToken := false
	foundArrowContinuation := false

	for _, ln := range lines[whyIdx+1:] {
		if strings.Contains(ln, "Source:") {
			break
		}
		if strings.Contains(ln, "systemd (pid 1)") {
			foundFirstToken = true
			// The first line should not start with arrow prefix.
			trim := strings.TrimLeft(ln, " ")
			if strings.HasPrefix(trim, "→ ") {
				t.Fatalf("expected first why line not to start with arrow prefix, got: %q", ln)
			}
		}
		trim := strings.TrimLeft(ln, " ")
		if strings.HasPrefix(trim, "→ ") {
			foundArrowContinuation = true
		}
	}

	if !foundFirstToken {
		t.Fatalf("expected output to include first why token, got:\n%s", out)
	}
	if !foundArrowContinuation {
		t.Fatalf("expected at least one continuation line starting with arrow prefix, got:\n%s", out)
	}
}
