//go:build linux

package why

import (
	"context"
	"fmt"
	"os"
)

func readProcessEnv(pid int) ([]string, error) {
	return readProcessEnvWithContext(context.Background(), pid, defaultEnvMaxBytes, defaultEnvMaxVars)
}

func readProcessEnvWithContext(ctx context.Context, pid int, maxBytes, maxVars int) ([]string, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readEnvironFromReaderWithContext(ctx, f, maxBytes, maxVars)
}
