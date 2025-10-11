package process

import (
	"os"
	"sort"
	"syscall"

	"github.com/shirou/gopsutil/v3/process"
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

// Item represents a process in our list.
type Item struct {
	pid        int32
	executable string
	User       string
	Status     Status
}

// NewItem creates a new Item for testing purposes.
func NewItem(pid int, executable, user string) *Item {
	return &Item{
		pid:        int32(pid),
		executable: executable,
		User:       user,
		Status:     Alive,
	}
}

// Pid returns the process ID.
func (i *Item) Pid() int {
	return int(i.pid)
}

// Executable returns the process executable name.
func (i *Item) Executable() string {
	return i.executable
}

// GetProcesses returns a list of all processes, wrapped in our Item struct.
func GetProcesses() ([]*Item, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	items := make([]*Item, 0, len(procs))
	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			// Skip processes we can't get a name for
			continue
		}
		user, err := p.Username()
		if err != nil {
			// If we can't get the username, we can default it.
			user = "n/a"
		}

		items = append(items, &Item{
			pid:        p.Pid,
			executable: name,
			User:       user,
			Status:     Alive,
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
