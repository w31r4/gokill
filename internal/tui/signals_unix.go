//go:build !windows

package tui

import "syscall"

const (
	sigStop = syscall.SIGSTOP
	sigCont = syscall.SIGCONT
)
