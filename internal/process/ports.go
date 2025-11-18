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

// getProcessPorts 是一个兼容性辅助函数，它为端口扫描操作设置一个默认的超时。
// 它内部调用了带有上下文（context）的 `getProcessPortsCtx` 函数，这是为了确保
// 对单个进程的端口扫描不会无限期地阻塞，从而影响整体应用的响应性。
func getProcessPorts(p *process.Process) []uint32 {
	// 使用 `context.WithTimeout` 创建一个带有超时机制的上下文。
	// 超时时长由 `portScanTimeout()` 函数决定，该函数可以读取环境变量进行配置。
	ctx, cancel := context.WithTimeout(context.Background(), portScanTimeout())
	// `defer cancel()` 确保在函数返回时，相关的上下文资源能够被及时释放，
	// 这是一个良好的编程习惯，可以防止上下文泄漏。
	defer cancel()
	return getProcessPortsCtx(ctx, p)
}

// getProcessPortsCtx 是实际执行端口采集的核心函数。
// 它接收一个上下文（`context.Context`）和一个进程对象（`*process.Process`），
// 返回该进程正在监听的所有端口号的唯一、有序列表。
func getProcessPortsCtx(ctx context.Context, p *process.Process) []uint32 {
	// 调用 `gopsutil` 库的 `ConnectionsWithContext` 方法获取进程的所有网络连接。
	// 传入的 `ctx` 参数使得这个 potentially long-running 操作可以被中断（例如，因超时）。
	conns, err := p.ConnectionsWithContext(ctx)
	if err != nil {
		// 如果在获取连接时发生错误（如权限问题或进程已退出），则返回 nil。
		return nil
	}

	// 使用 map 来存储唯一的端口号，`struct{}` 作为值是一个零字节的占位符，
	// 这种方式比使用 `map[uint32]bool` 更节省内存。
	unique := make(map[uint32]struct{})
	for _, conn := range conns {
		// 我们只关心本地地址的端口。如果端口号为0，则忽略。
		if conn.Laddr.Port == 0 {
			continue
		}
		// 核心逻辑：筛选出处于“监听”状态的连接。
		// `gopsutil` 库在不同操作系统或特定连接上可能返回 "NONE" 或空字符串作为状态，
		// 因此我们采取防御性措施，将这两种情况也视作监听状态，以避免遗漏潜在的监听端口。
		if conn.Status != "LISTEN" && conn.Status != "NONE" && conn.Status != "" {
			continue
		}
		// 将有效的监听端口号存入 map。
		unique[conn.Laddr.Port] = struct{}{}
	}

	// 如果没有找到任何监听端口，直接返回 nil。
	if len(unique) == 0 {
		return nil
	}

	// 将 map 中的唯一端口号转换成一个切片。
	ports := make([]uint32, 0, len(unique))
	for port := range unique {
		ports = append(ports, port)
	}

	// 对端口号进行升序排序，以确保每次显示的顺序都是一致和可预测的。
	sort.Slice(ports, func(i, j int) bool {
		return ports[i] < ports[j]
	})

	return ports
}

// formatPorts 是一个简单的工具函数，用于将一个 `[]uint32` 类型的端口列表
// 转换成一个人类可读的、用逗号和空格分隔的字符串。
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

// shouldScanPorts 函数通过检查环境变量 `GOKILL_SCAN_PORTS` 来决定是否应该执行端口扫描。
// 这是一个功能开关，允许用户根据需要禁用端口扫描，因为在某些系统上或在处理大量进程时，
// 端口扫描可能会有性能开销或需要特定权限。
//
// 默认行为 (环境变量未设置): 启用扫描。
// 显式启用: 环境变量值为 "1", "true", 或 "yes" (不区分大小写)。
// 显式禁用: 任何其他非空值。
func shouldScanPorts() bool {
	v := os.Getenv("GOKILL_SCAN_PORTS")
	// 如果环境变量未设置，默认为 true (启用)。
	if v == "" {
		return true
	}
	// 将环境变量的值转换为小写，以便进行不区分大小写的比较。
	s := strings.ToLower(v)
	return s == "1" || s == "true" || s == "yes"
}

// portScanTimeout 函数用于获取单个进程端口扫描的超时时间。
// 它允许用户通过环境变量 `GOKILL_PORT_TIMEOUT_MS` 来定制这个值，从而在不同的系统环境
// 或网络条件下进行微调，以平衡扫描的彻底性和应用的响应速度。
func portScanTimeout() time.Duration {
	v := os.Getenv("GOKILL_PORT_TIMEOUT_MS")
	// 如果环境变量未设置，返回一个合理的默认值 300 毫秒。
	if v == "" {
		return 300 * time.Millisecond
	}
	// 简易的纯数字解析，以避免引入 `strconv` 的潜在错误处理。
	// 如果值包含任何非数字字符，则立即回退到默认值。
	var ms int
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return 300 * time.Millisecond
		}
	}
	// 使用 `fmt.Sscanf` 将字符串解析为整数。
	_, err := fmt.Sscanf(v, "%d", &ms)
	// 如果解析失败或解析出的值小于等于0，同样回退到默认值。
	if err != nil || ms <= 0 {
		return 300 * time.Millisecond
	}
	// 将解析出的毫秒数转换为 `time.Duration` 类型并返回。
	return time.Duration(ms) * time.Millisecond
}
