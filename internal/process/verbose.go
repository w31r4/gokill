package process

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

const (
	verboseCollectTimeout = 900 * time.Millisecond
	verboseCacheTTL       = 3 * time.Second
	verboseCacheMax       = 64

	verboseMaxListenerBindings = 24
	verboseMaxChildren         = 12
)

type verboseCacheKey struct {
	pid        int32
	createTime int64
}

type verboseCacheEntry struct {
	lines     []string
	expiresAt time.Time
}

var verboseCache = struct {
	mu    sync.Mutex
	items map[verboseCacheKey]verboseCacheEntry
}{
	items: make(map[verboseCacheKey]verboseCacheEntry),
}

func appendVerboseSection(b *strings.Builder, p *process.Process, ports []uint32, hasPublicListener bool, opts DetailsOptions) {
	if !opts.Verbose || p == nil {
		return
	}

	key := verboseCacheKey{pid: p.Pid}
	{
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
		if createTime, err := p.CreateTimeWithContext(ctx); err == nil {
			key.createTime = createTime
		}
		cancel()
	}

	now := time.Now()
	if lines, ok := getVerboseCacheLines(key, now); ok {
		writeVerboseSection(b, lines)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), verboseCollectTimeout)
	lines := collectVerboseLines(ctx, p, ports, hasPublicListener)
	cancel()

	putVerboseCacheLines(key, lines, now)
	writeVerboseSection(b, lines)
}

func writeVerboseSection(b *strings.Builder, lines []string) {
	fmt.Fprintf(b, "\n  Verbose:\n")
	if len(lines) == 0 {
		fmt.Fprintf(b, "  (unavailable)\n")
		return
	}
	for _, line := range lines {
		fmt.Fprintf(b, "  %s\n", line)
	}
}

func getVerboseCacheLines(key verboseCacheKey, now time.Time) ([]string, bool) {
	verboseCache.mu.Lock()
	defer verboseCache.mu.Unlock()

	entry, ok := verboseCache.items[key]
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		delete(verboseCache.items, key)
		return nil, false
	}
	return entry.lines, true
}

func putVerboseCacheLines(key verboseCacheKey, lines []string, now time.Time) {
	verboseCache.mu.Lock()
	defer verboseCache.mu.Unlock()

	if verboseCacheMax > 0 && len(verboseCache.items) >= verboseCacheMax {
		for k, v := range verboseCache.items {
			if now.After(v.expiresAt) {
				delete(verboseCache.items, k)
			}
		}
		for len(verboseCache.items) >= verboseCacheMax {
			for k := range verboseCache.items {
				delete(verboseCache.items, k)
				break
			}
		}
	}

	verboseCache.items[key] = verboseCacheEntry{
		lines:     lines,
		expiresAt: now.Add(verboseCacheTTL),
	}
}

func collectVerboseLines(ctx context.Context, p *process.Process, ports []uint32, hasPublicListener bool) []string {
	lines := make([]string, 0, 6)
	lines = append(lines, verboseListenerLine(ctx, p))
	lines = append(lines, verboseMemoryLine(ctx, p))
	lines = append(lines, verboseIOLine(ctx, p))
	lines = append(lines, verboseFDLine(ctx, p))
	lines = append(lines, verboseThreadsLine(ctx, p))
	lines = append(lines, verboseChildrenLine(ctx, p))
	return lines
}

func verboseListenerLine(ctx context.Context, p *process.Process) string {
	if !shouldScanPorts() {
		return "Listen:\t(disabled: set GOKILL_SCAN_PORTS=1)"
	}

	conns, err := p.ConnectionsWithContext(ctx)
	if err != nil {
		return "Listen:\t" + unavailable(err)
	}

	unique := make(map[string]struct{})
	for _, conn := range conns {
		if conn.Laddr.Port == 0 {
			continue
		}
		if !isListenConnStatus(conn.Status) {
			continue
		}

		ip := strings.TrimSpace(conn.Laddr.IP)
		if ip == "" {
			ip = "*"
		} else if ip == ":::" {
			ip = "::"
		}
		unique[formatIPPort(ip, conn.Laddr.Port)] = struct{}{}
	}

	if len(unique) == 0 {
		return "Listen:\t(none)"
	}

	addrs := make([]string, 0, len(unique))
	for addr := range unique {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	limit := len(addrs)
	if limit > verboseMaxListenerBindings {
		limit = verboseMaxListenerBindings
	}

	val := strings.Join(addrs[:limit], " • ")
	if len(addrs) > limit {
		val = fmt.Sprintf("%s … (+%d)", val, len(addrs)-limit)
	}
	return "Listen:\t" + val
}

func isListenConnStatus(status string) bool {
	switch status {
	case "LISTEN", "NONE", "":
		return true
	default:
		return false
	}
}

func formatIPPort(ip string, port uint32) string {
	if strings.Contains(ip, ":") && !strings.HasPrefix(ip, "[") {
		return fmt.Sprintf("[%s]:%d", ip, port)
	}
	return fmt.Sprintf("%s:%d", ip, port)
}

func verboseIOLine(ctx context.Context, p *process.Process) string {
	io, err := p.IOCountersWithContext(ctx)
	if err != nil || io == nil {
		return "IO:\t" + unavailable(err)
	}
	return fmt.Sprintf(
		"IO:\tRead %s (%d) • Write %s (%d)",
		formatBytesIEC(io.ReadBytes),
		io.ReadCount,
		formatBytesIEC(io.WriteBytes),
		io.WriteCount,
	)
}

func verboseFDLine(ctx context.Context, p *process.Process) string {
	fds, err := p.NumFDsWithContext(ctx)
	if err != nil || fds < 0 {
		return "FDs:\t" + unavailable(err)
	}

	soft, hard := uint64(0), uint64(0)
	if rlimits, rErr := p.RlimitWithContext(ctx); rErr == nil {
		for _, r := range rlimits {
			if r.Resource != process.RLIMIT_NOFILE {
				continue
			}
			soft, hard = r.Soft, r.Hard
			break
		}
	}

	val := fmt.Sprintf("%d", fds)
	if soft > 0 || hard > 0 {
		val = fmt.Sprintf("%s (limit %d/%d)", val, soft, hard)
	}
	return "FDs:\t" + val
}

func verboseThreadsLine(ctx context.Context, p *process.Process) string {
	threads, err := p.NumThreadsWithContext(ctx)
	if err != nil || threads < 0 {
		return "Threads:\t" + unavailable(err)
	}
	return fmt.Sprintf("Threads:\t%d", threads)
}

func verboseChildrenLine(ctx context.Context, p *process.Process) string {
	children, err := p.ChildrenWithContext(ctx)
	if err != nil {
		return "Children:\t" + unavailable(err)
	}
	if len(children) == 0 {
		return "Children:\t0"
	}

	pids := make([]int32, 0, len(children))
	childrenByPID := make(map[int32]*process.Process, len(children))
	for _, ch := range children {
		if ch == nil {
			continue
		}
		pids = append(pids, ch.Pid)
		childrenByPID[ch.Pid] = ch
	}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })

	limit := len(pids)
	if limit > verboseMaxChildren {
		limit = verboseMaxChildren
	}

	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		pid := pids[i]
		part := fmt.Sprintf("%d", pid)
		if ch := childrenByPID[pid]; ch != nil {
			if name, nErr := ch.NameWithContext(ctx); nErr == nil && strings.TrimSpace(name) != "" {
				part = fmt.Sprintf("%d(%s)", pid, strings.TrimSpace(name))
			}
		}
		parts = append(parts, part)
	}

	val := strings.Join(parts, ", ")
	if len(pids) > limit {
		val = fmt.Sprintf("%s … (+%d)", val, len(pids)-limit)
	}
	return "Children:\t" + val
}

func unavailable(err error) string {
	if err == nil {
		return "(unavailable)"
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "(unavailable)"
	}
	return "(unavailable: " + truncateRunes(msg, 90) + ")"
}

func formatBytesIEC(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}

	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}

	value := float64(b) / float64(div)
	suffix := "KMGTPE"[exp]
	return fmt.Sprintf("%.1f %ciB", value, suffix)
}
