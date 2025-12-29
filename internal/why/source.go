package why

import (
	"context"
	"strings"
)

// detectSource attempts to identify the source/supervisor of a process.
// It checks the ancestry chain for known supervisors.
func detectSource(ctx context.Context, ancestry []ProcessInfo) Source {
	if len(ancestry) == 0 {
		return Source{Type: SourceUnknown, Confidence: 0.0}
	}

	// Check for known sources in order of specificity
	// More specific sources (PM2, Docker) take precedence over generic ones (systemd, shell)

	// Check for PM2
	if src := detectPM2(ancestry); src != nil {
		return *src
	}

	// Check for Docker (placeholder - will be enhanced in Phase 2)
	// if src := detectDocker(ancestry); src != nil {
	// 	return *src
	// }

	// Check for systemd (Linux only)
	if src := detectSystemdFromAncestry(ancestry); src != nil {
		return *src
	}

	// Check for launchd (macOS only)
	if src := detectLaunchdFromAncestry(ancestry); src != nil {
		return *src
	}

	// Check for shell (interactive terminal)
	if src := detectShell(ancestry); src != nil {
		return *src
	}

	return Source{Type: SourceUnknown, Confidence: 0.2}
}

// detectPM2 checks if the process is managed by PM2.
func detectPM2(ancestry []ProcessInfo) *Source {
	for _, p := range ancestry {
		cmd := strings.ToLower(p.Command)
		cmdline := strings.ToLower(p.Cmdline)

		if strings.Contains(cmd, "pm2") || strings.Contains(cmdline, "pm2") {
			return &Source{
				Type:       SourcePM2,
				Name:       "pm2",
				Confidence: 0.9,
			}
		}
	}
	return nil
}

// detectSystemdFromAncestry checks for systemd in the ancestry.
func detectSystemdFromAncestry(ancestry []ProcessInfo) *Source {
	if len(ancestry) == 0 {
		return nil
	}

	// Check if the root is systemd (PID 1 with command "systemd")
	root := ancestry[0]
	if root.PID == 1 && strings.Contains(strings.ToLower(root.Command), "systemd") {
		// If direct child of systemd, likely a service
		if len(ancestry) >= 2 && ancestry[1].PPID == 1 {
			return &Source{
				Type:       SourceSystemd,
				Name:       ancestry[1].Command,
				Confidence: 0.8,
			}
		}
		return &Source{
			Type:       SourceSystemd,
			Confidence: 0.6,
		}
	}
	return nil
}

// detectLaunchdFromAncestry checks for launchd in the ancestry.
func detectLaunchdFromAncestry(ancestry []ProcessInfo) *Source {
	if len(ancestry) == 0 {
		return nil
	}

	// Check if the root is launchd (PID 1 with command "launchd")
	root := ancestry[0]
	if root.PID == 1 && strings.Contains(strings.ToLower(root.Command), "launchd") {
		// If direct child of launchd, likely a service
		if len(ancestry) >= 2 && ancestry[1].PPID == 1 {
			return &Source{
				Type:       SourceLaunchd,
				Name:       ancestry[1].Command,
				Confidence: 0.8,
			}
		}
		return &Source{
			Type:       SourceLaunchd,
			Confidence: 0.6,
		}
	}
	return nil
}

// detectShell checks if the process was started from an interactive shell.
func detectShell(ancestry []ProcessInfo) *Source {
	shells := map[string]bool{
		"bash":  true,
		"zsh":   true,
		"fish":  true,
		"sh":    true,
		"dash":  true,
		"tcsh":  true,
		"csh":   true,
		"ksh":   true,
		"ash":   true,
		"login": true,
	}

	for _, p := range ancestry {
		cmd := strings.ToLower(p.Command)
		// Remove path prefix if present
		if idx := strings.LastIndex(cmd, "/"); idx >= 0 {
			cmd = cmd[idx+1:]
		}
		// Remove leading dash (login shell indicator)
		cmd = strings.TrimPrefix(cmd, "-")

		if shells[cmd] {
			return &Source{
				Type:       SourceShell,
				Name:       p.Command,
				Confidence: 0.7,
			}
		}
	}
	return nil
}
