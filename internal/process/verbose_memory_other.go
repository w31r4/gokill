//go:build !linux

package process

import (
	"context"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

func verboseMemoryLine(ctx context.Context, p *process.Process) string {
	mem, err := p.MemoryInfoWithContext(ctx)
	if err != nil || mem == nil {
		return "Memory:\t" + unavailable(err)
	}

	var parts []string
	if mem.RSS > 0 {
		parts = append(parts, "RSS "+formatBytesIEC(mem.RSS))
	}
	if mem.VMS > 0 {
		parts = append(parts, "VMS "+formatBytesIEC(mem.VMS))
	}
	if len(parts) == 0 {
		return "Memory:\t(n/a)"
	}
	return "Memory:\t" + strings.Join(parts, " • ")
}
