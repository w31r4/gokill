package process

import "testing"

func TestShouldRedactEnvKey(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"PASSWORD", true},
		{"DB_PASSWORD", true},
		{"AWS_SECRET_ACCESS_KEY", true},
		{"GITHUB_TOKEN", true},
		{"API_KEY", true},
		{"MY_PASS", true},
		{"KEYBOARD_LAYOUT", false},
		{"PATH", false},
		{"HOME", false},
		{"PWD", false},
		{"XDG_SESSION_TYPE", false},
	}

	for _, tc := range cases {
		if got := shouldRedactEnvKey(tc.key); got != tc.want {
			t.Fatalf("shouldRedactEnvKey(%q)=%v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestFormatEnvEntryRedaction(t *testing.T) {
	entry := "DB_PASSWORD=supersecret"

	redacted := formatEnvEntry(entry, false)
	if redacted == entry {
		t.Fatalf("expected redaction, got: %q", redacted)
	}
	if wantSub := "<redacted>"; !contains(redacted, wantSub) {
		t.Fatalf("expected %q in output, got: %q", wantSub, redacted)
	}
	if contains(redacted, "supersecret") {
		t.Fatalf("expected secret not to appear, got: %q", redacted)
	}

	revealed := formatEnvEntry(entry, true)
	if revealed != entry {
		t.Fatalf("expected revealed output to match input, got: %q", revealed)
	}
}

func TestSanitizeEnvEntryEscapesAndTruncates(t *testing.T) {
	entry := "X=1\n2\t3\r4\x1b[31mRED"
	out := sanitizeEnvEntry(entry)
	if contains(out, "\n") || contains(out, "\t") || contains(out, "\r") || contains(out, "\x1b") {
		t.Fatalf("expected output to be escaped, got: %q", out)
	}

	long := "A=" + repeat("x", 400)
	out2 := sanitizeEnvEntry(long)
	if len([]rune(out2)) > 220 {
		t.Fatalf("expected output to be truncated to <= 220 runes, got %d", len([]rune(out2)))
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	// Small helper to avoid importing strings in tests.
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
