//go:build linux

package why

import (
	"fmt"
	"os"
)

func readProcessEnv(pid int) ([]string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return nil, err
	}
	return parseEnvironBytes(data), nil
}

