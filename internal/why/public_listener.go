package why

import (
	"context"
	"os"
	"strings"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
	ps "github.com/shirou/gopsutil/v3/process"
)

const publicListenerWarning = "Process is listening on a public interface (0.0.0.0/::)"

func shouldScanPorts() bool {
	v := os.Getenv("GOKILL_SCAN_PORTS")
	if v == "" {
		return true
	}
	s := strings.ToLower(v)
	return s == "1" || s == "true" || s == "yes"
}

func detectPublicListener(pid int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	return detectPublicListenerCtx(ctx, pid)
}

func detectPublicListenerCtx(ctx context.Context, pid int) bool {
	if !shouldScanPorts() {
		return false
	}

	p, err := ps.NewProcess(int32(pid))
	if err != nil {
		return false
	}

	conns, err := p.ConnectionsWithContext(ctx)
	if err != nil {
		return false
	}

	return hasPublicListenerFromConns(conns)
}

func hasPublicListenerFromConns(conns []gnet.ConnectionStat) bool {
	for _, conn := range conns {
		if conn.Laddr.Port == 0 {
			continue
		}

		// Only consider listeners.
		if conn.Status != "LISTEN" && conn.Status != "NONE" && conn.Status != "" {
			continue
		}

		ip := conn.Laddr.IP
		// Be conservative: some platforms may omit IP for listeners.
		if ip == "" || ip == "0.0.0.0" || ip == "::" || ip == ":::" {
			return true
		}
	}
	return false
}
