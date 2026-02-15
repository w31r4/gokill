package process

import (
	"testing"
)

func TestParseContainerIPFromCmdline(t *testing.T) {
	tests := []struct {
		name     string
		cmdline  string
		expected string
	}{
		{
			name:     "standard docker-proxy cmdline",
			cmdline:  "/usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 6185 -container-ip 172.17.0.2 -container-port 6185",
			expected: "172.17.0.2",
		},
		{
			name:     "container-ip at end",
			cmdline:  "docker-proxy -container-ip 10.0.0.5",
			expected: "10.0.0.5",
		},
		{
			name:     "no container-ip flag",
			cmdline:  "docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 8080",
			expected: "",
		},
		{
			name:     "empty cmdline",
			cmdline:  "",
			expected: "",
		},
		{
			name:     "container-ip without value",
			cmdline:  "docker-proxy -container-ip",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseContainerIPFromCmdline(tt.cmdline)
			if got != tt.expected {
				t.Errorf("parseContainerIPFromCmdline(%q) = %q, want %q", tt.cmdline, got, tt.expected)
			}
		})
	}
}

func TestParseNetworkInspectOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []containerEntry
	}{
		{
			name:   "single container",
			output: "nginx:172.17.0.2/16\n",
			expected: []containerEntry{
				{Name: "nginx", IPv4: "172.17.0.2"},
			},
		},
		{
			name:   "multiple containers",
			output: "nginx:172.17.0.2/16\nredis:172.17.0.3/16\nastrbot:172.17.0.4/16\n",
			expected: []containerEntry{
				{Name: "nginx", IPv4: "172.17.0.2"},
				{Name: "redis", IPv4: "172.17.0.3"},
				{Name: "astrbot", IPv4: "172.17.0.4"},
			},
		},
		{
			name:     "empty output",
			output:   "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			output:   "  \n  \n",
			expected: nil,
		},
		{
			name:     "malformed line without colon",
			output:   "noColonHere\n",
			expected: nil,
		},
		{
			name:   "mixed valid and empty lines",
			output: "\nnginx:172.17.0.2/16\n\nredis:172.17.0.3/16\n",
			expected: []containerEntry{
				{Name: "nginx", IPv4: "172.17.0.2"},
				{Name: "redis", IPv4: "172.17.0.3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNetworkInspectOutput(tt.output)
			if len(got) != len(tt.expected) {
				t.Fatalf("parseNetworkInspectOutput: got %d entries, want %d", len(got), len(tt.expected))
			}
			for i, e := range tt.expected {
				if got[i].Name != e.Name || got[i].IPv4 != e.IPv4 {
					t.Errorf("entry[%d]: got {%s, %s}, want {%s, %s}", i, got[i].Name, got[i].IPv4, e.Name, e.IPv4)
				}
			}
		})
	}
}

func TestDockerNetworkResolverCaching(t *testing.T) {
	// Test that the resolver's cache mechanism works correctly
	r := newDockerNetworkResolver()

	// Pre-populate cache to simulate inspected networks
	r.cache["bridge"] = []containerEntry{
		{Name: "nginx", IPv4: "172.17.0.2"},
		{Name: "redis", IPv4: "172.17.0.3"},
	}
	r.cache["my-app-net"] = []containerEntry{
		{Name: "astrbot", IPv4: "172.18.0.2"},
	}

	// Verify cache hits
	if entries := r.getOrInspect("bridge"); len(entries) != 2 {
		t.Errorf("expected 2 cached bridge entries, got %d", len(entries))
	}
	if entries := r.getOrInspect("my-app-net"); len(entries) != 1 {
		t.Errorf("expected 1 cached my-app-net entry, got %d", len(entries))
	}
	if entries := r.getOrInspect("my-app-net"); entries[0].Name != "astrbot" {
		t.Errorf("expected cached entry name 'astrbot', got %q", entries[0].Name)
	}
}

func TestNewDockerNetworkResolverFreshCache(t *testing.T) {
	// Each new resolver should have an empty cache (scan-scoped freshness)
	r1 := newDockerNetworkResolver()
	r1.cache["bridge"] = []containerEntry{{Name: "stale", IPv4: "10.0.0.1"}}

	r2 := newDockerNetworkResolver()
	if len(r2.cache) != 0 {
		t.Errorf("new resolver should have empty cache, got %d entries", len(r2.cache))
	}
}
