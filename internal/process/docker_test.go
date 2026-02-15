package process

import "testing"

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
