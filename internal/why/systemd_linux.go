//go:build linux

package why

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func resolveSystemdUnit(ctx context.Context, pid int) string {
	if pid <= 0 {
		return ""
	}

	if unit := systemdUnitFromCgroup(pid, "/"); unit != "" {
		return unit
	}

	// Fallback: ask systemctl for the unit owning this PID.
	if _, err := exec.LookPath("systemctl"); err != nil {
		return ""
	}

	cmdCtx := ctx
	if cmdCtx == nil {
		cmdCtx = context.Background()
	}
	cmdCtx, cancel := context.WithTimeout(cmdCtx, 600*time.Millisecond)
	defer cancel()

	out, _ := exec.CommandContext(cmdCtx, "systemctl", "status", "--no-pager", "--full", strconv.Itoa(pid)).CombinedOutput()
	return findFirstUnitToken(string(out), ".service")
}

func systemdUnitFromCgroup(pid int, rootPath string) string {
	if rootPath == "" {
		rootPath = "/"
	}
	rootPath = filepath.Clean(rootPath)

	data, err := os.ReadFile(filepath.Join(rootPath, "proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return ""
	}
	return extractSystemdUnitFromCgroupContent(string(data))
}

func extractSystemdUnitFromCgroupContent(content string) string {
	var candidates []string

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// cgroup v1: <hier>:<controllers>:<path>
		// cgroup v2: 0::<path>
		idx := strings.LastIndex(line, ":")
		if idx == -1 || idx == len(line)-1 {
			continue
		}
		path := line[idx+1:]

		// Prefer the deepest unit in the path. For systemd services, this is typically the last
		// *.service segment.
		unit := ""
		for _, seg := range strings.Split(path, "/") {
			if strings.HasSuffix(seg, ".service") {
				unit = seg
			}
		}
		if unit != "" {
			candidates = append(candidates, unit)
		}
	}

	return pickBestSystemdUnitCandidate(candidates)
}

func pickBestSystemdUnitCandidate(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}

	// Prefer units that are not the user manager itself (user@UID.service) when a more specific
	// unit exists (e.g., app.service).
	for i := len(candidates) - 1; i >= 0; i-- {
		if !strings.HasPrefix(candidates[i], "user@") {
			return candidates[i]
		}
	}
	return candidates[len(candidates)-1]
}

func findFirstUnitToken(text string, suffix string) string {
	if text == "" || suffix == "" {
		return ""
	}
	for _, line := range strings.Split(text, "\n") {
		for _, tok := range strings.Fields(line) {
			tok = strings.TrimSpace(tok)
			tok = strings.TrimPrefix(tok, "●")
			tok = strings.Trim(tok, "();,")
			if strings.HasSuffix(tok, suffix) {
				return tok
			}
		}
	}
	return ""
}

