//go:build linux

package why

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// readProcessInfo reads process information from /proc filesystem.
func readProcessInfo(ctx context.Context, pid int, includeWorkingDir bool) (ProcessInfo, error) {
	info := ProcessInfo{PID: pid}

	if err := ctx.Err(); err != nil {
		return info, err
	}

	// Read /proc/[pid]/stat for basic info
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return info, err
	}

	// Parse stat file - format is complex due to command name in parentheses
	statStr := string(statData)
	openParen := strings.Index(statStr, "(")
	closeParen := strings.LastIndex(statStr, ")")
	if openParen == -1 || closeParen == -1 {
		return info, fmt.Errorf("invalid stat format")
	}

	// Command name is between parentheses
	info.Command = statStr[openParen+1 : closeParen]

	// Fields after the closing parenthesis
	fields := strings.Fields(statStr[closeParen+2:])
	if len(fields) < 22 {
		return info, fmt.Errorf("insufficient stat fields")
	}

	// Field 0: State (R, S, Z, etc.)
	info.Status = fields[0]

	// Field 1 (0-indexed after command): PPID
	ppid, _ := strconv.Atoi(fields[1])
	info.PPID = ppid

	// Field 11/12: utime/stime (clock ticks)
	utimeTicks, _ := strconv.ParseInt(fields[11], 10, 64)
	stimeTicks, _ := strconv.ParseInt(fields[12], 10, 64)
	if hz := ticksPerSecond(); hz > 0 {
		info.CPUTime = time.Duration(utimeTicks+stimeTicks) * time.Second / time.Duration(hz)
	}

	// Field 19: starttime (in clock ticks since boot)
	startTicks, _ := strconv.ParseInt(fields[19], 10, 64)
	if hz := ticksPerSecond(); hz > 0 {
		info.StartedAt = bootTime().Add(time.Duration(startTicks) * time.Second / time.Duration(hz))
	}

	// Field 21: rss (in pages)
	rssPages, _ := strconv.ParseUint(fields[21], 10, 64)
	info.RSS = rssPages * uint64(os.Getpagesize())

	// Read username
	info.User = readUser(pid)

	// Read command line
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	if cmdlineData, err := os.ReadFile(cmdlinePath); err == nil {
		// Replace null bytes with spaces
		cmdline := strings.ReplaceAll(string(cmdlineData), "\x00", " ")
		info.Cmdline = strings.TrimSpace(cmdline)
	}

	// Read working directory (target process only)
	if includeWorkingDir {
		cwdPath := fmt.Sprintf("/proc/%d/cwd", pid)
		if cwd, err := os.Readlink(cwdPath); err == nil {
			info.WorkingDir = cwd
		}
	}

	return info, nil
}

// getProcessStartTimePlatform returns the process start time as Unix timestamp.
func getProcessStartTimePlatform(pid int) int64 {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return 0
	}

	statStr := string(statData)
	closeParen := strings.LastIndex(statStr, ")")
	if closeParen == -1 {
		return 0
	}

	fields := strings.Fields(statStr[closeParen+2:])
	if len(fields) < 20 {
		return 0
	}

	startTicks, _ := strconv.ParseInt(fields[19], 10, 64)
	if hz := ticksPerSecond(); hz > 0 {
		return bootTime().Add(time.Duration(startTicks) * time.Second / time.Duration(hz)).Unix()
	}
	return 0
}

// readUser reads the username for a process.
func readUser(pid int) string {
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return "unknown"
	}

	// Find Uid line
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				uid, _ := strconv.Atoi(fields[1])
				return uidToUsername(uid)
			}
		}
	}
	return "unknown"
}

// uidToUsername converts UID to username.
func uidToUsername(uid int) string {
	// Read /etc/passwd to find username
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return strconv.Itoa(uid)
	}

	uidStr := strconv.Itoa(uid)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 3 && fields[2] == uidStr {
			return fields[0]
		}
	}
	return uidStr
}

// bootTime returns the system boot time.
var (
	bootTimeCache time.Time
	bootTimeMu    sync.Mutex
)

func bootTime() time.Time {
	bootTimeMu.Lock()
	defer bootTimeMu.Unlock()

	if !bootTimeCache.IsZero() {
		return bootTimeCache
	}

	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				btime, _ := strconv.ParseInt(fields[1], 10, 64)
				bootTimeCache = time.Unix(btime, 0)
				return bootTimeCache
			}
		}
	}
	return time.Time{}
}

// ticksPerSecond returns the system clock ticks per second.
func ticksPerSecond() int64 {
	// Typical value on Linux (USER_HZ). Best-effort, sufficient for coarse health warnings.
	return 100
}
