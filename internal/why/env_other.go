//go:build !linux

package why

import "fmt"

func readProcessEnv(pid int) ([]string, error) {
	return nil, fmt.Errorf("environment variable reading is not implemented on this platform")
}

