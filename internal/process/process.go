package process

import (
	"os"
	"sort"
	"syscall"

	"github.com/mitchellh/go-ps"
)

// Process is a wrapper around ps.Process
type Process ps.Process

// GetProcesses returns a list of all processes sorted by name.
func GetProcesses() ([]Process, error) {
	procs, err := ps.Processes()
	if err != nil {
		return nil, err
	}

	sort.Slice(procs, func(i, j int) bool {
		return procs[i].Executable() < procs[j].Executable()
	})

	result := make([]Process, len(procs))
	for i, p := range procs {
		result[i] = Process(p)
	}

	return result, nil
}

// KillProcess kills a process by its PID.
func KillProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}
