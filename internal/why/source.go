package why

import (
	"context"
	"strings"
)

// detectSource attempts to identify the source/supervisor of a process.
// It checks the ancestry chain for known supervisors.
func detectSource(ctx context.Context, ancestry []ProcessInfo) Source {
	_ = ctx

	if len(ancestry) == 0 {
		return Source{Type: SourceUnknown, Confidence: 0.0}
	}

	// Collect candidates and pick the best one using a stable priority order,
	// then confidence as a tie-breaker.
	var candidates []*Source

	// Phase 2 detectors (best-effort; may return nil on unsupported platforms)
	if src := detectContainer(ancestry, ""); src != nil {
		candidates = append(candidates, src)
	}
	if src := detectSupervisor(ancestry); src != nil {
		candidates = append(candidates, src)
	}
	if src := detectCron(ancestry); src != nil {
		candidates = append(candidates, src)
	}

	// Existing detectors
	if src := detectSystemdFromAncestry(ancestry); src != nil {
		candidates = append(candidates, src)
	}
	if src := detectLaunchdFromAncestry(ancestry); src != nil {
		candidates = append(candidates, src)
	}
	if src := detectShell(ancestry); src != nil {
		candidates = append(candidates, src)
	}

	if best := pickBestSource(candidates); best != nil {
		return *best
	}

	return Source{Type: SourceUnknown, Confidence: 0.2}
}

func pickBestSource(candidates []*Source) *Source {
	typePriority := map[SourceType]int{
		SourceDocker:     0,
		SourcePM2:        1,
		SourceSupervisor: 1,
		SourceCron:       2,
		SourceSystemd:    3,
		SourceLaunchd:    3,
		SourceShell:      4,
		SourceUnknown:    5,
	}

	var best *Source
	bestPriority := typePriority[SourceUnknown]

	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		priority, ok := typePriority[candidate.Type]
		if !ok {
			priority = typePriority[SourceUnknown]
		}

		if best == nil || priority < bestPriority || (priority == bestPriority && candidate.Confidence > best.Confidence) {
			best = candidate
			bestPriority = priority
		}
	}

	return best
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
