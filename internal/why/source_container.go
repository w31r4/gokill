package why

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// detectContainer checks if the process or any of its ancestors are running inside a container.
// It inspects /proc/<pid>/cgroup for container signatures.
// rootPath is used for testing purposes (default should be "").
func detectContainer(ancestry []ProcessInfo, rootPath string) *Source {
	if rootPath == "" {
		rootPath = "/"
	}
	rootPath = filepath.Clean(rootPath)

	// Prefer the target process first (last element in ancestry), then walk upward.
	for idx := len(ancestry) - 1; idx >= 0; idx-- {
		p := ancestry[idx]
		path := filepath.Join(rootPath, "proc", strconv.Itoa(p.PID), "cgroup")

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)

		// Normalize: we only expose a single container SourceType/Name for stable UX/API.
		if strings.Contains(content, "docker") ||
			strings.Contains(content, "containerd") ||
			strings.Contains(content, "kubepods") ||
			strings.Contains(content, "lxc") {
			containerID := extractContainerID(content)
			name := "container"
			if containerID != "" {
				name = containerID
			}
			return &Source{
				Type:       SourceDocker,
				Name:       name,
				Confidence: 0.9,
			}
		}
	}
	return nil
}

func extractContainerID(content string) string {
	best := ""
	segments := strings.FieldsFunc(content, func(r rune) bool {
		return r == '\n' || r == '/' || r == ':' || r == ' '
	})
	for _, seg := range segments {
		id := normalizeContainerSegment(seg)
		if id == "" {
			continue
		}
		if len(id) > len(best) {
			best = id
		}
	}
	return best
}

func normalizeContainerSegment(seg string) string {
	if seg == "" {
		return ""
	}
	seg = strings.TrimPrefix(seg, "docker-")
	seg = strings.TrimPrefix(seg, "cri-containerd-")
	seg = strings.TrimSuffix(seg, ".scope")
	seg = strings.TrimSpace(seg)
	if len(seg) < 8 {
		return ""
	}
	if !isHexString(seg) {
		return ""
	}
	return seg
}

func isHexString(s string) bool {
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return true
}
