//go:build !linux

package why

func isProcessExeDeleted(pid int) bool {
	return false
}

