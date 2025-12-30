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
		// Additional info can be added later via Source.Details when needed.
		if strings.Contains(content, "docker") ||
			strings.Contains(content, "containerd") ||
			strings.Contains(content, "kubepods") ||
			strings.Contains(content, "lxc") {
			return &Source{
				Type:       SourceDocker,
				Name:       "container",
				Confidence: 0.9,
			}
		}
	}
	return nil
}
