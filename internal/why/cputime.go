package why

import (
	"strconv"
	"strings"
	"time"
)

// parsePsCPUTime parses macOS/Linux ps-style CPU time fields like:
// - MM:SS
// - HH:MM:SS
// - DD-HH:MM:SS
// It returns 0 on parse failure.
func parsePsCPUTime(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	days := int64(0)
	if dash := strings.IndexByte(s, '-'); dash > 0 {
		d, err := strconv.ParseInt(s[:dash], 10, 64)
		if err != nil || d < 0 {
			return 0
		}
		days = d
		s = s[dash+1:]
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	nums := make([]int64, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return 0
		}
		nums = append(nums, n)
	}
	hours := int64(0)
	minutes := int64(0)
	seconds := int64(0)
	if len(nums) == 2 {
		minutes = nums[0]
		seconds = nums[1]
	} else {
		hours = nums[0]
		minutes = nums[1]
		seconds = nums[2]
	}
	totalSeconds := (((days*24)+hours)*60+minutes)*60 + seconds
	if totalSeconds < 0 {
		return 0
	}
	return time.Duration(totalSeconds) * time.Second
}
