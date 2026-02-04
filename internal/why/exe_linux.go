//go:build linux

package why

import (
	"fmt"
	"os"
	"strings"
)

func isProcessExeDeleted(pid int) bool {
	exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return false
	}
	return strings.HasSuffix(exePath, " (deleted)")
}

