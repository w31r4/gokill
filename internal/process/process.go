package process

import (
	"os"
	"os/user"
	"sort"
	"syscall"

	"github.com/mitchellh/go-ps"
)

// Status represents the state of a process item in the list.
type Status int

const (
	// Alive is the default status for a running process.
	Alive Status = iota
	// Killed marks a process that has been sent a SIGTERM signal.
	Killed
	// Paused marks a process that has been sent a SIGSTOP signal.
	Paused
)

// Item represents a process in our list. It wraps the original ps.Process
// and adds our own state management.
type Item struct {
	ps.Process
	Status Status
	UID    string
	User   string
}

// GetProcesses returns a list of all processes, wrapped in our Item struct.
func GetProcesses() ([]*Item, error) {
	procs, err := ps.Processes()
	if err != nil {
		return nil, err
	}

	items := make([]*Item, 0, len(procs))
	currentUser, err := user.Current()
	if err != nil {
		return nil, err
	}

	for _, p := range procs {
		items = append(items, &Item{
			Process: p,
			Status:  Alive,
			User:    currentUser.Username,
		})
	}

	// Sort by executable name
	sort.Slice(items, func(i, j int) bool {
		return items[i].Executable() < items[j].Executable()
	})

	return items, nil
}

// SendSignal sends a signal to a process by its PID.
func SendSignal(pid int, sig syscall.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}
