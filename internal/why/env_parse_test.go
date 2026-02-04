package why

import "testing"

func TestParseEnvironBytes(t *testing.T) {
	got := parseEnvironBytes([]byte("A=1\x00B=2\x00"))
	if len(got) != 2 || got[0] != "A=1" || got[1] != "B=2" {
		t.Fatalf("unexpected result: %#v", got)
	}

	if gotEmpty := parseEnvironBytes(nil); gotEmpty != nil {
		t.Fatalf("expected nil for empty input, got %#v", gotEmpty)
	}
}

