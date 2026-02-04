//go:build linux

package why

import "testing"

func TestExtractSystemdUnitFromCgroupContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "SimpleSystemServiceV2",
			content: "0::/system.slice/ssh.service\n",
			want:    "ssh.service",
		},
		{
			name:    "SimpleSystemServiceV1",
			content: "1:name=systemd:/system.slice/nginx.service\n",
			want:    "nginx.service",
		},
		{
			name:    "UserServicePrefersAppOverUserManager",
			content: "0::/user.slice/user-1000.slice/user@1000.service/app.slice/emacs.service\n",
			want:    "emacs.service",
		},
		{
			name:    "OnlyUserManagerService",
			content: "0::/user.slice/user-1000.slice/user@1000.service\n",
			want:    "user@1000.service",
		},
		{
			name:    "NoServiceUnit",
			content: "0::/user.slice/user-1000.slice/session-2.scope\n",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractSystemdUnitFromCgroupContent(tt.content); got != tt.want {
				t.Fatalf("extractSystemdUnitFromCgroupContent()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindFirstUnitToken(t *testing.T) {
	// A representative first line for `systemctl status <pid>`.
	out := "● ssh.service - OpenBSD Secure Shell server\n   Loaded: loaded (/lib/systemd/system/ssh.service; enabled)\n"
	if got := findFirstUnitToken(out, ".service"); got != "ssh.service" {
		t.Fatalf("findFirstUnitToken()=%q, want %q", got, "ssh.service")
	}
}

