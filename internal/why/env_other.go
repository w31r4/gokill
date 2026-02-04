//go:build !linux

package why

import (
	"context"
	"fmt"
)

func readProcessEnv(pid int) ([]string, error) {
	return nil, fmt.Errorf("environment variable reading is not implemented on this platform")
}

func readProcessEnvWithContext(ctx context.Context, pid int, maxBytes, maxVars int) ([]string, error) {
	return readProcessEnv(pid)
}
