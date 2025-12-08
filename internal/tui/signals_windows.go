//go:build windows

package tui

import "syscall"

// Windows doesn't have SIGSTOP/SIGCONT in the same way Unix does.
// We'll use 0 as a placeholder, and the process package should handle
// the actual suspension/resumption logic for Windows if implemented,
// or gracefully fail/ignore.
const (
	sigStop = syscall.Signal(0)
	sigCont = syscall.Signal(0)
)
