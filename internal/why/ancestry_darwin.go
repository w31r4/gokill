//go:build darwin

package why

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// readProcessInfo reads process information using ps command on macOS.
func readProcessInfo(pid int) (ProcessInfo, error) {
	info := ProcessInfo{PID: pid}

	// Use ps to get process info
	// Format: pid,ppid,user,state,rss,lstart,comm,command
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "pid=,ppid=,user=,state=,rss=,lstart=,comm=,args=")
	output, err := cmd.Output()
	if err != nil {
		return info, err
	}

	// Parse output
	line := strings.TrimSpace(string(output))
	if line == "" {
		return info, err
	}

	// Parse fields - this is tricky because lstart has spaces
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return info, nil
	}

	// PID (should match)
	info.PID, _ = strconv.Atoi(fields[0])

	// PPID
	info.PPID, _ = strconv.Atoi(fields[1])

	// User
	info.User = fields[2]

	// State
	info.Status = fields[3]

	// RSS (in KB)
	rssKB, _ := strconv.ParseUint(fields[4], 10, 64)
	info.RSS = rssKB * 1024

	// lstart format: "Day Mon DD HH:MM:SS YYYY" (5 fields)
	// Example: "Sun Dec 29 10:15:30 2024"
	// Fields 5-9 are lstart
	if len(fields) >= 10 {
		lstartStr := strings.Join(fields[5:10], " ")
		if t, err := time.Parse("Mon Jan 2 15:04:05 2006", lstartStr); err == nil {
			info.StartedAt = t
		}
	}

	// Command name (field 10)
	if len(fields) >= 11 {
		info.Command = fields[10]
	}

	// Full command line (remaining fields)
	if len(fields) > 11 {
		info.Cmdline = strings.Join(fields[11:], " ")
	} else if len(fields) == 11 {
		info.Cmdline = fields[10]
	}

	// Get working directory using lsof
	info.WorkingDir = getWorkingDir(pid)

	return info, nil
}

// getWorkingDir gets the working directory using lsof.
func getWorkingDir(pid int) string {
	cmd := exec.Command("lsof", "-p", strconv.Itoa(pid), "-Fn", "-a", "-d", "cwd")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse lsof output - look for line starting with 'n'
	for _, line := range bytes.Split(output, []byte("\n")) {
		if len(line) > 1 && line[0] == 'n' {
			return string(line[1:])
		}
	}
	return ""
}

// getProcessStartTimePlatform returns the process start time as Unix timestamp.
func getProcessStartTimePlatform(pid int) int64 {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lstartStr := strings.TrimSpace(string(output))
	if t, err := time.Parse("Mon Jan 2 15:04:05 2006", lstartStr); err == nil {
		return t.Unix()
	}
	return 0
}
