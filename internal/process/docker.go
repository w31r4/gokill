package process

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// parseContainerIPFromCmdline extracts the -container-ip value from a
// docker-proxy command line. Returns "" if not found.
func parseContainerIPFromCmdline(cmdline string) string {
	parts := strings.Fields(cmdline)
	for i, part := range parts {
		if part == "-container-ip" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// containerEntry represents a container discovered on a Docker network.
type containerEntry struct {
	Name string
	IPv4 string
}

// dockerNetworkResolver resolves docker-proxy container IPs to container names
// by inspecting all Docker networks. Results are cached within a single scan cycle
// to avoid redundant CLI calls from concurrent workers.
type dockerNetworkResolver struct {
	mu    sync.Mutex
	cache map[string][]containerEntry // network name → containers
}

// newDockerNetworkResolver creates a fresh resolver for one GetProcesses() scan cycle.
func newDockerNetworkResolver() *dockerNetworkResolver {
	return &dockerNetworkResolver{
		cache: make(map[string][]containerEntry),
	}
}

// listDockerNetworks returns the names of all Docker networks.
func listDockerNetworks() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "network", "ls", "--format", "{{.Name}}").Output()
	if err != nil {
		return nil
	}

	var networks []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			networks = append(networks, line)
		}
	}
	return networks
}

// inspectNetwork queries Docker for containers on a given network and returns
// their name→IP mappings.
func inspectNetwork(name string) []containerEntry {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "network", "inspect", name,
		"--format", `{{range .Containers}}{{.Name}}:{{.IPv4Address}}{{"\\n"}}{{end}}`).Output()
	if err != nil {
		return nil
	}

	return parseNetworkInspectOutput(string(out))
}

// parseNetworkInspectOutput parses the output of `docker network inspect`.
// Each line is in the format "containerName:IP/mask".
func parseNetworkInspectOutput(output string) []containerEntry {
	var entries []containerEntry
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}
		name := line[:colonIdx]
		ip := strings.Split(line[colonIdx+1:], "/")[0]
		if name != "" && ip != "" {
			entries = append(entries, containerEntry{Name: name, IPv4: ip})
		}
	}
	return entries
}

// resolve maps a docker-proxy cmdline to the container name by searching
// across all Docker networks. It caches network inspection results so that
// multiple docker-proxy processes in one scan don't trigger redundant calls.
func (r *dockerNetworkResolver) resolve(cmdline string) string {
	containerIP := parseContainerIPFromCmdline(cmdline)
	if containerIP == "" {
		return ""
	}

	networks := listDockerNetworks()
	if len(networks) == 0 {
		return ""
	}

	for _, net := range networks {
		entries := r.getOrInspect(net)
		for _, e := range entries {
			if e.IPv4 == containerIP {
				return e.Name
			}
		}
	}
	return ""
}

// getOrInspect returns cached entries for a network, or inspects it and caches the result.
// Thread-safe for concurrent worker access.
func (r *dockerNetworkResolver) getOrInspect(network string) []containerEntry {
	r.mu.Lock()
	if entries, ok := r.cache[network]; ok {
		r.mu.Unlock()
		return entries
	}
	r.mu.Unlock()

	// Inspect without holding the lock (CLI call is slow).
	entries := inspectNetwork(network)

	r.mu.Lock()
	// Double-check: another goroutine may have filled it while we were inspecting.
	if cached, ok := r.cache[network]; ok {
		r.mu.Unlock()
		return cached
	}
	r.cache[network] = entries
	r.mu.Unlock()
	return entries
}

// StopContainer stops a Docker container by name with a timeout.
func StopContainer(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "stop", name).Run()
}
