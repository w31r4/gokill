package process

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/w31r4/gokill/internal/why"
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
	PPid       int32    `json:"ppid"`
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
		PPid:       0,
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
//  1. **扇出 (Fan-out)**: 主 Goroutine 获取所有进程列表，然后将每个进程作为一个“任务”发送到 `jobs` channel 中。
//  2. **Worker Pool**: 程序会根据系统的 CPU 核心数创建一组（`numWorkers`）Goroutine。这些 Goroutine 被称为 "Worker"。
//     它们每一个都同时从 `jobs` channel 中取出任务进行处理。处理过程包括获取进程的详细信息（如名称、用户、端口等），
//     这是一个相对耗时的 I/O 操作，因此非常适合并发执行。
//  3. **扇入 (Fan-in)**: 每个 Worker 完成任务后，会将处理结果（一个 `Item` 结构体）发送到 `results` channel 中。
//     这样，所有 Worker 的处理结果都汇集到了同一个 channel。
//  4. **同步 (Synchronization)**: 主 Goroutine 使用 `sync.WaitGroup` 来等待所有的 Worker 都完成它们的工作。
//     这是确保在收集结果之前，所有任务都已经被处理完毕的关键。
//  5. **收集 (Collection)**: 所有 Worker 都结束后，主 Goroutine 从 `results` channel 中读取所有的结果，
//     并将它们汇总到一个切片（slice）中，最后进行排序并返回。
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
	// I/O 密集型（连接采集）适当提高并发度，取 CPU*2 与进程数的较小值。
	numCPU := runtime.NumCPU()
	numWorkers := numCPU * 2
	if numWorkers > len(procs) {
		numWorkers = len(procs)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}
	// 创建一个带缓冲的 channel 用于存放待处理的进程任务。
	// 缓冲大小设置为进程总数，这样主 Goroutine 可以一次性将所有任务放入 channel 而不会阻塞。
	jobs := make(chan *process.Process, len(procs))
	// 创建另一个带缓冲的 channel 用于收集处理完成的结果。
	results := make(chan *Item, len(procs))
	// 创建一个用于收集非致命错误（警告）的 channel。
	// 改为小容量并在后台聚合，避免当每个进程产生多条警告时阻塞 worker。
	warnings := make(chan error, runtime.NumCPU())

	// 后台聚合所有警告，防止 warnings 写满造成阻塞。
	var warnWG sync.WaitGroup
	warnWG.Add(1)
	collectedWarnings := make([]error, 0, len(procs))
	go func() {
		defer warnWG.Done()
		for w := range warnings {
			collectedWarnings = append(collectedWarnings, w)
		}
	}()

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
			// 是否扫描端口由环境变量控制，避免每次全量扫描带来的性能/权限问题。
			scanPorts := shouldScanPorts()

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

				ppid, err := p.Ppid()
				if err != nil {
					ppid = 0
					warnings <- fmt.Errorf("pid %d: failed to get ppid: %w", p.Pid, err)
				}

				// 获取该进程监听的端口号（可选，带超时）。
				var ports []uint32
				if scanPorts {
					// 为单个进程的连接采集设定一个短超时，避免卡顿拖慢整体。
					ctx, cancel := context.WithTimeout(context.Background(), portScanTimeout())
					ports, _ = getProcessListenerInfoCtx(ctx, p)
					cancel()
				}

				// --- 任务完成，发送结果 ---
				// 将处理好的进程信息封装成 Item 结构体，并发送到 `results` channel。
				results <- &Item{
					Pid:        p.Pid,
					PPid:       ppid,
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

	// 等待后台聚合器读取完所有 warnings
	warnWG.Wait()

	// 最后，对结果进行排序，以便在界面上更友好
	sort.Slice(items, func(i, j int) bool {
		return items[i].Executable < items[j].Executable
	})

	// 返回最终处理好的进程列表和所有警告
	return items, collectedWarnings, nil
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

	details := collectProcessDetails(p)
	ports, hasPublicListener := scanProcessPorts(p)

	var b strings.Builder
	writeBaseDetails(&b, details, ports)
	appendWhySection(&b, pid, p, ports, hasPublicListener)

	return b.String(), nil
}

type processDetails struct {
	user       string
	name       string
	cpuPercent float64
	memPercent float32
	startTime  string
	pid        int32
	exe        string
	cmdline    string
}

func collectProcessDetails(p *process.Process) processDetails {
	return processDetails{
		user:       fetchUsername(p),
		name:       fetchName(p),
		cpuPercent: sampleCPUPercent(p),
		memPercent: fetchMemoryPercent(p),
		startTime:  fetchStartTime(p),
		pid:        p.Pid,
		exe:        fetchExe(p),
		cmdline:    fetchCmdline(p),
	}
}

func fetchUsername(p *process.Process) string {
	user, err := p.Username()
	if err != nil {
		return "n/a"
	}
	return user
}

func sampleCPUPercent(p *process.Process) float64 {
	// 第一次采样常为 0；进行一次短间隔的二次采样以获得更可信的数值。
	if v, err := p.CPUPercent(); err == nil && v > 0 {
		return v
	}
	time.Sleep(200 * time.Millisecond)
	if v2, err2 := p.CPUPercent(); err2 == nil {
		return v2
	}
	return 0.0
}

func fetchMemoryPercent(p *process.Process) float32 {
	memPercent, err := p.MemoryPercent()
	if err != nil {
		return 0.0
	}
	return memPercent
}

func fetchStartTime(p *process.Process) string {
	createTime, err := p.CreateTime() // returns millis since epoch
	if err != nil {
		return "n/a"
	}
	return time.Unix(createTime/1000, 0).Format("Jan 02 15:04")
}

func fetchName(p *process.Process) string {
	name, err := p.Name()
	if err != nil {
		return "n/a"
	}
	return name
}

func fetchExe(p *process.Process) string {
	exe, err := p.Exe()
	if err != nil {
		return "(permission denied or n/a)"
	}
	return exe
}

func fetchCmdline(p *process.Process) string {
	cmdline, err := p.Cmdline()
	if err != nil || cmdline == "" {
		return "(n/a)"
	}
	return cmdline
}

func scanProcessPorts(p *process.Process) ([]uint32, bool) {
	if !shouldScanPorts() {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), portScanTimeout())
	defer cancel()
	ports, public := getProcessListenerInfoCtx(ctx, p)
	return ports, public
}

func writeBaseDetails(b *strings.Builder, details processDetails, ports []uint32) {
	fmt.Fprintf(b, "  User:\t%s\n", details.user)
	fmt.Fprintf(b, "  Name:\t%s\n", details.name)
	fmt.Fprintf(b, "  %%CPU:\t%.1f\n", details.cpuPercent)
	fmt.Fprintf(b, "  %%MEM:\t%.1f\n", details.memPercent)
	fmt.Fprintf(b, "  Start:\t%s\n", details.startTime)
	if len(ports) > 0 {
		fmt.Fprintf(b, "  Ports:\t%s\n", formatPorts(ports))
	}
	fmt.Fprintf(b, "  Target:\t%s\n", formatTargetSummary(details.name, int(details.pid), ports))
	fmt.Fprintf(b, "  PID:\t%d\n", details.pid)
	fmt.Fprintf(b, "  Exe:\t%s\n", details.exe)
	fmt.Fprintf(b, "  Command:\t%s\n", details.cmdline)
}

func appendWhySection(b *strings.Builder, pid int, p *process.Process, ports []uint32, hasPublicListener bool) {
	// Analyze process ancestry and source with a 2 second timeout.
	result, _ := why.AnalyzeWithTimeout(pid, 2*time.Second)
	if result == nil {
		return
	}

	writeWhyHeader(b)
	appendAncestryChain(b, result)
	appendSourceDetails(b, result)
	appendWorkingDir(b, result)
	appendGitDetails(b, result)
	fmt.Fprintf(b, "  Restart Count:\t%d\n", result.RestartCount)
	appendContextSection(b, p, ports, hasPublicListener)
	appendWarningsSection(b, result, hasPublicListener)
	appendEnvSection(b, result)
	writeWhyFooter(b)
}

func writeWhyHeader(b *strings.Builder) {
	fmt.Fprintf(b, "\n  ─────────────────────────────────────\n")
	fmt.Fprintf(b, "  Why It Exists:\n")
}

func writeWhyFooter(b *strings.Builder) {
	fmt.Fprintf(b, "  ─────────────────────────────────────\n")
}

func appendAncestryChain(b *strings.Builder, result *why.AnalysisResult) {
	if len(result.Ancestry) == 0 {
		return
	}
	fmt.Fprintf(b, "    %s\n", why.FormatAncestryChain(result.Ancestry))
}

func appendSourceDetails(b *strings.Builder, result *why.AnalysisResult) {
	sourceLine := formatSourceLine(result.Source)
	fmt.Fprintf(b, "\n  Source:\t%s\n", sourceLine)
	if result.SystemdUnit != "" {
		fmt.Fprintf(b, "  Service:\t%s\n", result.SystemdUnit)
	} else if needsServiceLine(result.Source) {
		fmt.Fprintf(b, "  Service:\t%s\n", result.Source.Name)
	}
	if result.ContainerID != "" {
		fmt.Fprintf(b, "  Container:\t%s\n", result.ContainerID)
	}
}

func formatSourceLine(source why.Source) string {
	sourceStr := string(source.Type)
	if sourceStr == "" {
		sourceStr = string(why.SourceUnknown)
	}
	sourceName := source.Name
	if source.Type == why.SourceSystemd || source.Type == why.SourceLaunchd || source.Type == why.SourceDocker {
		sourceName = ""
	}
	if sourceName != "" && sourceName != sourceStr {
		return fmt.Sprintf("%s (%s)", sourceName, sourceStr)
	}
	return sourceStr
}

func needsServiceLine(source why.Source) bool {
	if source.Name == "" {
		return false
	}
	return source.Type == why.SourceSystemd || source.Type == why.SourceLaunchd
}

func appendWorkingDir(b *strings.Builder, result *why.AnalysisResult) {
	if result.WorkingDir == "" {
		return
	}
	fmt.Fprintf(b, "  Working Dir:\t%s\n", result.WorkingDir)
}

func appendGitDetails(b *strings.Builder, result *why.AnalysisResult) {
	if result.GitRepo == "" {
		return
	}
	if result.GitBranch != "" {
		fmt.Fprintf(b, "  Git Repo:\t%s (%s)\n", result.GitRepo, result.GitBranch)
		return
	}
	fmt.Fprintf(b, "  Git Repo:\t%s\n", result.GitRepo)
}

func appendContextSection(b *strings.Builder, p *process.Process, ports []uint32, hasPublicListener bool) {
	contextLines := buildContextLines(p, ports, hasPublicListener)
	if len(contextLines) == 0 {
		return
	}
	fmt.Fprintf(b, "\n  Context:\n")
	for _, line := range contextLines {
		fmt.Fprintf(b, "  %s\n", line)
	}
}

func appendWarningsSection(b *strings.Builder, result *why.AnalysisResult, hasPublicListener bool) {
	warnings := result.Warnings
	if hasPublicListener {
		warnings = append(warnings, "Process is listening on a public interface (0.0.0.0/::)")
	}
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintf(b, "\n  Warnings:\n")
	for _, warning := range warnings {
		fmt.Fprintf(b, "  ⚠ %s\n", warning)
	}
}

func appendEnvSection(b *strings.Builder, result *why.AnalysisResult) {
	if result == nil {
		return
	}
	if len(result.Env) == 0 && result.EnvError == "" {
		return
	}

	fmt.Fprintf(b, "\n  Env:\n")

	if len(result.Env) == 0 {
		fmt.Fprintf(b, "  (unavailable: %s)\n", strings.TrimSpace(result.EnvError))
		return
	}

	// Keep the output stable and bounded.
	env := append([]string(nil), result.Env...)
	sort.Strings(env)

	const maxEnvLines = 30
	limit := len(env)
	if limit > maxEnvLines {
		limit = maxEnvLines
	}

	for i := 0; i < limit; i++ {
		fmt.Fprintf(b, "  %s\n", sanitizeEnvEntry(env[i]))
	}
	if len(env) > maxEnvLines {
		fmt.Fprintf(b, "  … (%d more)\n", len(env)-maxEnvLines)
	}
}

func sanitizeEnvEntry(s string) string {
	// Make env display safe for our line-based formatter.
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\x1b", "") // strip ANSI ESC
	return s
}

func formatTargetSummary(name string, pid int, ports []uint32) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || trimmed == "n/a" {
		trimmed = "process"
	}
	summary := fmt.Sprintf("%s (pid %d)", trimmed, pid)
	if len(ports) > 0 {
		summary = fmt.Sprintf("%s, port %d", summary, ports[0])
		if len(ports) > 1 {
			summary = fmt.Sprintf("%s (+%d)", summary, len(ports)-1)
		}
	}
	return summary
}

func buildContextLines(p *process.Process, ports []uint32, hasPublicListener bool) []string {
	var lines []string

	if state := formatSocketState(ports, hasPublicListener); state != "" {
		lines = append(lines, fmt.Sprintf("Socket State:\t%s", state))
	}

	if resource := formatResourceContext(p); resource != "" {
		lines = append(lines, fmt.Sprintf("Resource:\t%s", resource))
	}

	if files := formatFileContext(p); files != "" {
		lines = append(lines, fmt.Sprintf("Files:\t%s", files))
	}

	return lines
}

func formatSocketState(ports []uint32, hasPublicListener bool) string {
	if len(ports) == 0 {
		return ""
	}
	state := fmt.Sprintf("listening %d", len(ports))
	if hasPublicListener {
		state += " (public)"
	}
	return state
}

func formatResourceContext(p *process.Process) string {
	var parts []string

	if memInfo, err := p.MemoryInfo(); err == nil && memInfo != nil && memInfo.RSS > 0 {
		rssMB := float64(memInfo.RSS) / (1024 * 1024)
		parts = append(parts, fmt.Sprintf("RSS %.1f MB", rssMB))
	}

	if threads, err := p.NumThreads(); err == nil && threads > 0 {
		parts = append(parts, fmt.Sprintf("Threads %d", threads))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " • ")
}

func formatFileContext(p *process.Process) string {
	files, err := p.OpenFiles()
	if err != nil || len(files) == 0 {
		return ""
	}
	return fmt.Sprintf("Open %d", len(files))
}
