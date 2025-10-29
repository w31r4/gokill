package process

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"

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
	StartTime  string
	Status     Status
	ports      []uint32
}

// NewItem creates a new Item for testing purposes.
func NewItem(pid int, executable, user string, ports ...int) *Item {
	portList := make([]uint32, 0, len(ports))
	for _, port := range ports {
		if port <= 0 {
			continue
		}
		portList = append(portList, uint32(port))
	}
	if len(portList) > 1 {
		sort.Slice(portList, func(i, j int) bool {
			return portList[i] < portList[j]
		})
	}

	return &Item{
		pid:        int32(pid),
		executable: executable,
		User:       user,
		Status:     Alive,
		ports:      portList,
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

// Ports returns the list of ports the process is listening on.
func (i *Item) Ports() []uint32 {
	return i.ports
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

		createTime, err := p.CreateTime()
		startTime := "n/a"
		if err == nil {
			startTime = time.Unix(createTime/1000, 0).Format("15:04:05")
		}

		ports := getProcessPorts(p)

		items = append(items, &Item{
			pid:        p.Pid,
			executable: name,
			User:       user,
			StartTime:  startTime,
			Status:     Alive,
			ports:      ports,
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

// GetProcessDetails returns detailed information about a process.
func GetProcessDetails(pid int) (string, error) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return "", fmt.Errorf("process with pid %d not found: %w", pid, err)
	}

	user, err := p.Username()
	if err != nil {
		user = "n/a"
	}

	cpuPercent, err := p.CPUPercent()
	if err != nil {
		// On the first call, it may return 0.0.
		cpuPercent = 0.0
	}

	memPercent, err := p.MemoryPercent()
	if err != nil {
		memPercent = 0.0
	}

	createTime, err := p.CreateTime() // returns millis since epoch
	startTime := "n/a"
	if err == nil {
		startTime = time.Unix(createTime/1000, 0).Format("Jan 02 15:04")
	}

	cmdline, err := p.Cmdline()
	if err != nil || cmdline == "" {
		cmdline, _ = p.Name()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "  PID:\t%d\n", p.Pid)
	fmt.Fprintf(&b, "  User:\t%s\n", user)
	fmt.Fprintf(&b, "  %%CPU:\t%.1f\n", cpuPercent)
	fmt.Fprintf(&b, "  %%MEM:\t%.1f\n", memPercent)
	fmt.Fprintf(&b, "  Start:\t%s\n", startTime)
	if ports := getProcessPorts(p); len(ports) > 0 {
		fmt.Fprintf(&b, "  Ports:\t%s\n", formatPorts(ports))
	}
	fmt.Fprintf(&b, "  Command:\t%s\n", cmdline)

	return b.String(), nil
}

func getProcessPorts(p *process.Process) []uint32 {
	conns, err := p.Connections()
	if err != nil {
		return nil
	}

	unique := make(map[uint32]struct{})
	for _, conn := range conns {
		if conn.Laddr.Port == 0 {
			continue
		}
		if conn.Status != "LISTEN" && conn.Status != "NONE" && conn.Status != "" {
			continue
		}
		unique[conn.Laddr.Port] = struct{}{}
	}

	if len(unique) == 0 {
		return nil
	}

	ports := make([]uint32, 0, len(unique))
	for port := range unique {
		ports = append(ports, port)
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i] < ports[j]
	})

	return ports
}

func formatPorts(ports []uint32) string {
	if len(ports) == 0 {
		return ""
	}

	parts := make([]string, len(ports))
	for i, port := range ports {
		parts[i] = fmt.Sprintf("%d", port)
	}

	return strings.Join(parts, ", ")
}
