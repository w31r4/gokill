package process

import (
	"context"
	"os/exec"
	"strings"
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

// resolveDockerProxyContainer attempts to map a docker-proxy process's
// command line to the Docker container name it forwards traffic for.
// It extracts the container IP from the cmdline, then queries the Docker
// bridge network to find which container owns that IP.
func resolveDockerProxyContainer(cmdline string) string {
	containerIP := parseContainerIPFromCmdline(cmdline)
	if containerIP == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "network", "inspect", "bridge",
		"--format", `{{range .Containers}}{{.Name}}:{{.IPv4Address}}{{"\\n"}}{{end}}`).Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}
		name := line[:colonIdx]
		ip := strings.Split(line[colonIdx+1:], "/")[0]
		if ip == containerIP {
			return name
		}
	}
	return ""
}

// StopContainer stops a Docker container by name with a timeout.
func StopContainer(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "stop", name).Run()
}
