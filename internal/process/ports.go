package process

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// 兼容原有调用：默认使用配置的端口扫描超时。
func getProcessPorts(p *process.Process) []uint32 {
	ctx, cancel := context.WithTimeout(context.Background(), portScanTimeout())
	defer cancel()
	return getProcessPortsCtx(ctx, p)
}

// getProcessPortsCtx 带 context 的端口采集，支持超时/取消。
func getProcessPortsCtx(ctx context.Context, p *process.Process) []uint32 {
	conns, err := p.ConnectionsWithContext(ctx)
	if err != nil {
		return nil
	}

	unique := make(map[uint32]struct{})
	for _, conn := range conns {
		if conn.Laddr.Port == 0 {
			continue
		}
		// We are only interested in listening ports.
		// The gopsutil library may return "NONE" or an empty string for the status
		// on some platforms for certain connections, so we include them as a
		// defensive measure to avoid missing potentially relevant ports.
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

// shouldScanPorts 通过环境变量控制是否扫描端口。
// 当 GOKILL_SCAN_PORTS 为空、"1"、"true"、"yes" 时启用扫描；其他值则禁用。
func shouldScanPorts() bool {
	v := os.Getenv("GOKILL_SCAN_PORTS")
	if v == "" {
		return true
	}
	s := strings.ToLower(v)
	return s == "1" || s == "true" || s == "yes"
}

// portScanTimeout 返回端口扫描的超时时间，默认 300ms。
// 可通过环境变量 GOKILL_PORT_TIMEOUT_MS 覆盖（整数毫秒）。
func portScanTimeout() time.Duration {
	v := os.Getenv("GOKILL_PORT_TIMEOUT_MS")
	if v == "" {
		return 300 * time.Millisecond
	}
	// 简易解析，失败则回退默认。
	var ms int
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return 300 * time.Millisecond
		}
	}
	_, err := fmt.Sscanf(v, "%d", &ms)
	if err != nil || ms <= 0 {
		return 300 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

