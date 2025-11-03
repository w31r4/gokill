package process

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
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
	Pid        int32    `json:"pid"`
	Executable string   `json:"executable"`
	User       string   `json:"user"`
	StartTime  string   `json:"startTime"`
	Status     Status   `json:"status"`
	Ports      []uint32 `json:"ports"`
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
		Pid:        int32(pid),
		Executable: executable,
		User:       user,
		Status:     Alive,
		Ports:      portList,
	}
}

// GetProcesses 获取系统中所有正在运行的进程，并将它们封装成我们自定义的 Item 结构体列表。
// 这个函数是整个应用的性能关键点之一，因为它需要处理可能成百上千个进程。
// 为了高效地处理这些进程信息，这里采用了一个经典的并发模式：扇出/扇入（Fan-out/Fan-in）的 Worker Pool 模式。
//
// 并发模型详解：
// 1. **扇出 (Fan-out)**: 主 Goroutine 获取所有进程列表，然后将每个进程作为一个“任务”发送到 `jobs` channel 中。
// 2. **Worker Pool**: 程序会根据系统的 CPU 核心数创建一组（`numWorkers`）Goroutine。这些 Goroutine 被称为 "Worker"。
//    它们每一个都同时从 `jobs` channel 中取出任务进行处理。处理过程包括获取进程的详细信息（如名称、用户、端口等），
//    这是一个相对耗时的 I/O 操作，因此非常适合并发执行。
// 3. **扇入 (Fan-in)**: 每个 Worker 完成任务后，会将处理结果（一个 `Item` 结构体）发送到 `results` channel 中。
//    这样，所有 Worker 的处理结果都汇集到了同一个 channel。
// 4. **同步 (Synchronization)**: 主 Goroutine 使用 `sync.WaitGroup` 来等待所有的 Worker 都完成它们的工作。
//    这是确保在收集结果之前，所有任务都已经被处理完毕的关键。
// 5. **收集 (Collection)**: 所有 Worker 都结束后，主 Goroutine 从 `results` channel 中读取所有的结果，
//    并将它们汇总到一个切片（slice）中，最后进行排序并返回。
//
// 这种模式的优势在于，它将一个大的、可分解的任务（获取所有进程信息）分解成许多小任务，并利用多核 CPU 并行处理，
// 从而极大地缩短了总体的处理时间。
func GetProcesses() ([]*Item, []error, error) {
	// 首先，使用 gopsutil 库获取一个包含所有进程的列表。
	procs, err := process.Processes()
	if err != nil {
		// 如果获取失败，直接返回错误。
		return nil, nil, err
	}

	// **并发设置**
	// 根据当前机器的 CPU 核心数来决定启动多少个 Worker Goroutine。
	// 这样做可以充分利用 CPU 资源，同时避免创建过多 Goroutine 导致调度开销过大。
	numWorkers := runtime.NumCPU()
	// 创建一个带缓冲的 channel 用于存放待处理的进程任务。
	// 缓冲大小设置为进程总数，这样主 Goroutine 可以一次性将所有任务放入 channel 而不会阻塞。
	jobs := make(chan *process.Process, len(procs))
	// 创建另一个带缓冲的 channel 用于收集处理完成的结果。
	results := make(chan *Item, len(procs))
	// 创建一个 channel 用于收集非致命错误（警告）。
	warnings := make(chan error, len(procs))

	// **Worker Pool 的启动**
	// 使用 WaitGroup 来追踪所有 Worker Goroutine 的完成状态。
	var wg sync.WaitGroup
	// 这个循环创建并启动了 `numWorkers` 个 Worker Goroutine。
	for w := 0; w < numWorkers; w++ {
		// 每启动一个 Goroutine，WaitGroup 的计数器就加一。
		wg.Add(1)
		go func() {
			// 使用 defer 确保在 Goroutine 退出时，一定会调用 Done()，将 WaitGroup 计数器减一。
			// 这是至关重要的，否则主 Goroutine 可能会永远等待下去。
			defer wg.Done()

			// 每个 Worker 不断地从 `jobs` channel 中接收任务。
			// `for range` 会一直阻塞，直到 channel 被关闭并且所有值都被接收完毕。
			for p := range jobs {
				// --- 单个进程信息的处理 ---
				name, err := p.Name()
				if err != nil {
					// 如果获取进程名失败，则发送一个警告并跳过这个进程。
					warnings <- fmt.Errorf("pid %d: failed to get name: %w", p.Pid, err)
					continue
				}
				user, err := p.Username()
				if err != nil {
					user = "n/a" // 失败则使用默认值
					warnings <- fmt.Errorf("pid %d: failed to get user: %w", p.Pid, err)
				}

				createTime, err := p.CreateTime()
				startTime := "n/a"
				if err == nil {
					// 将毫秒级时间戳转换为格式化的字符串。
					startTime = time.Unix(createTime/1000, 0).Format("15:04:05")
				} else {
					warnings <- fmt.Errorf("pid %d: failed to get create time: %w", p.Pid, err)
				}

				// 获取该进程监听的端口号。
				ports := getProcessPorts(p)

				// --- 任务完成，发送结果 ---
				// 将处理好的进程信息封装成 Item 结构体，并发送到 `results` channel。
				results <- &Item{
					Pid:        p.Pid,
					Executable: name,
					User:       user,
					StartTime:  startTime,
					Status:     Alive,
					Ports:      ports,
				}
			}
		}()
	}

	// **任务的分发 (扇出)**
	// 主 Goroutine 遍历所有进程，并将它们逐一发送到 `jobs` channel。
	for _, p := range procs {
		jobs <- p
	}
	// 当所有任务都已发送完毕后，必须关闭 `jobs` channel。
	// 这是一个信号，告诉正在 `for range` 循环的 Worker Goroutine：“不会再有新的任务了”。
	// Worker 处理完 channel 中剩余的任务后，循环会自动结束，Goroutine 随之退出。
	close(jobs)

	// **等待与收集 (扇入)**
	// 主 Goroutine 在这里会阻塞，直到 WaitGroup 的计数器变为零。
	// 也就是说，它会一直等到所有 Worker Goroutine 都调用了 `wg.Done()`。
	wg.Wait()
	// 此时，可以确定所有的处理结果都已经被发送到了 `results` 和 `warnings` channel。
	// 关闭这些 channel，为下一步的接收做准备。
	close(results)
	close(warnings)

	// 从 `results` channel 中读取所有处理好的 `Item`。
	// `for range` 会遍历 channel 中所有的数据，直到 channel 被关闭且为空。
	items := make([]*Item, 0, len(procs))
	for item := range results {
		items = append(items, item)
	}

	allWarnings := make([]error, 0, len(warnings))
	for warning := range warnings {
		allWarnings = append(allWarnings, warning)
	}

	// 最后，对结果进行排序，以便在界面上更友好地展示。
	sort.Slice(items, func(i, j int) bool {
		return items[i].Executable < items[j].Executable
	})

	// 返回最终处理好的进程列表和所有警告。
	return items, allWarnings, nil
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
