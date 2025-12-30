package why

import (
	"fmt"
	"os"
	"strings"
)

// detectContainer checks if the process or any of its ancestors are running inside a container.
// It inspects /proc/<pid>/cgroup for container signatures.
// rootPath is used for testing purposes (default should be "").
func detectContainer(ancestry []ProcessInfo, rootPath string) *Source {
	if rootPath == "" {
		rootPath = "/"
	}
	// Normalize rootPath to ensure it ends with / if needed, or handle in path construction.
	// Actually simple string concat is fine if we are careful.
	// os.ReadFile takes a path.

	for _, p := range ancestry {
		// Construct path: rootPath + proc/<pid>/cgroup
		// If rootPath is "/", path is /proc/<pid>/cgroup
		// If rootPath is "/tmp/test", path is /tmp/test/proc/<pid>/cgroup
		path := fmt.Sprintf("%sproc/%d/cgroup", rootPath, p.PID)
		if rootPath == "/" {
			path = fmt.Sprintf("/proc/%d/cgroup", p.PID)
		} else {
			// Ensure cleanly joined
			path = strings.TrimRight(rootPath, "/") + fmt.Sprintf("/proc/%d/cgroup", p.PID)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)

		// Normalize: we only expose a single container SourceType/Name for stable UX/API.
		// Additional info can be added later via Source.Details when needed.
		if strings.Contains(content, "docker") || strings.Contains(content, "containerd") || strings.Contains(content, "kubepods") || strings.Contains(content, "lxc") {
			return &Source{
				Type:       SourceDocker,
				Name:       "container",
				Confidence: 0.9,
			}
		}
	}
	return nil
}
