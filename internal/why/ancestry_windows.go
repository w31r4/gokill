//go:build windows

package why

import (
	"context"
	"fmt"
)

// readProcessInfo is not implemented on Windows yet.
func readProcessInfo(ctx context.Context, pid int, includeWorkingDir bool) (ProcessInfo, error) {
	if err := ctx.Err(); err != nil {
		return ProcessInfo{PID: pid}, err
	}
	return ProcessInfo{PID: pid}, fmt.Errorf("process inspection not supported on windows")
}

// getProcessStartTimePlatform returns 0 on Windows (unsupported).
func getProcessStartTimePlatform(pid int) int64 {
	return 0
}
