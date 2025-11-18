package tui

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/w31r4/gokill/internal/process"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
)

// --- Bubble Tea 消息定义 ---
// Bubble Tea 框架通过消息（Message, `tea.Msg`）来驱动应用状态的更新。
// 消息可以是任何类型，通常是结构体或自定义类型。它们由命令（`tea.Cmd`）产生，或由框架自身因用户输入（如按键）而生成。

// processesLoadedMsg wraps the results of fetching processes, including any
// non-fatal errors (warnings) that occurred during the process.
type processesLoadedMsg struct {
	processes []*process.Item
	warnings  []error
}

// processDetailsMsg 用于携带单个进程的详细信息字符串。
type processDetailsMsg string

// errMsg 用于在发生错误时，将错误信息传递给 Update 方法进行处理和显示。
type errMsg struct{ err error }

// signalOKMsg 表示进程信号发送成功的反馈消息，用于在 UI 中更新状态。
type signalOKMsg struct {
	pid    int
	status process.Status
}

// --- 应用状态模型 ---
// model 结构体是 Bubble Tea 应用的核心，它包含了应用在任何时刻的所有状态。
// 它是UI的“单一数据源”（Single Source of Truth）。View 函数会根据这个 model 的数据来渲染界面，
// 而 Update 函数会根据接收到的消息来更新这个 model 的状态。
type model struct {
	// processes 存储从系统中获取的原始进程列表，相当于一个完整的缓存。
	processes []*process.Item
	// filtered 存储根据用户输入（搜索词）过滤后的进程列表，这是在界面上实际显示的列表。
	filtered []*process.Item
	// cursor 表示当前用户界面上光标选中的项目在 `filtered` 列表中的索引。
	cursor int
	// textInput 是一个来自 `bubbles/textinput` 库的组件，用于处理用户的文本输入。
	textInput textinput.Model
	// err 用于存储在应用运行过程中可能发生的错误，以便在界面上显示。
	err      error
	warnings []error
	// showDetails 是一个布尔标志，用于控制是显示进程列表还是显示单个进程的详细信息视图。
	showDetails bool
	// processDetails 存储从 `GetProcessDetails` 获取到的详细信息字符串。
	processDetails string
	// portsOnly 为 true 时，仅显示监听端口的进程。
	portsOnly bool

	// --- Dependency (T) mode state ---
	depMode          bool
	depRootPID       int32
	depExpanded      map[int32]depNodeState
	depCursor        int
	depShowAncestors bool

	// --- Confirm overlay ---
	confirm *confirmPrompt

	// Phase 4: filter and toggles for T-mode
	depAliveOnly bool
	depPortsOnly bool

	// help overlay
	helpOpen bool
}

// InitialModel 创建并返回应用的初始状态模型。
// 它在程序启动时被调用一次。
func InitialModel(filter string) model {
	ti := textinput.New()
	ti.Placeholder = "Search processes or ports"
	ti.CharLimit = 156
	ti.Width = 20
	ti.SetValue(filter)

	cached, _ := process.Load()

	m := model{
		textInput: ti,
		processes: cached,
	}
	m.filtered = m.filterProcesses(filter)
	return m
}

// --- 模糊搜索逻辑 ---
// 为了实现高效且用户友好的搜索功能，我们使用了 `sahilm/fuzzy` 库。
// 这个库需要一个实现了 `fuzzy.Source` 接口的数据源。
// `fuzzyProcessSource` 就是为我们的进程列表 `[]*process.Item` 实现这个接口的适配器。

// fuzzyProcessSource 包装了进程列表。
type fuzzyProcessSource struct {
	processes []*process.Item
}

// String 是 `fuzzy.Source` 接口要求的方法。
// 它返回在给定索引 `i` 处的项目的字符串表示形式，模糊搜索将在这个字符串上进行匹配。
// 为了让用户可以同时通过进程名、PID、用户名或端口号进行搜索，我们将这几项信息拼接成一个单一的字符串。
func (s fuzzyProcessSource) String(i int) string {
	p := s.processes[i]
	if ports := portsForSearch(p.Ports); ports != "" {
		return fmt.Sprintf("%s %s %d %s", p.Executable, p.User, p.Pid, ports)
	}
	return fmt.Sprintf("%s %s %d", p.Executable, p.User, p.Pid)
}

// Len 是 `fuzzy.Source` 接口要求的另一个方法，返回数据源中的项目总数。
func (s fuzzyProcessSource) Len() int {
	return len(s.processes)
}

// filterProcesses 根据给定的过滤字符串（filter）来筛选进程列表。
func (m *model) filterProcesses(filter string) []*process.Item {
	var filtered []*process.Item
	// 如果过滤字符串为空，我们不过滤，而是返回所有未被杀死的进程。
	if filter == "" {
		for _, p := range m.processes {
			if p.Status != process.Killed {
				if m.portsOnly && len(p.Ports) == 0 {
					continue
				}
				filtered = append(filtered, p)
			}
		}
		if m.portsOnly {
			sort.SliceStable(filtered, func(i, j int) bool {
				// 端口列表在采集时已升序，这里取第一个端口作为排序键
				return filtered[i].Ports[0] < filtered[j].Ports[0]
			})
		}
		return filtered
	}

	// 如果有过滤条件，则使用模糊搜索。
	// 1. 创建一个 `fuzzyProcessSource` 实例。
	source := fuzzyProcessSource{processes: m.processes}
	// 2. 调用 `fuzzy.FindFrom` 进行搜索，它会返回一个按匹配度排序的结果列表。
	matches := fuzzy.FindFrom(filter, source)

	// 3. 根据匹配结果，从原始的 `m.processes` 列表中构建出过滤后的列表。
	for _, match := range matches {
		p := m.processes[match.Index]
		// 同样，我们只包括未被杀死的进程。
		if p.Status != process.Killed {
			if m.portsOnly && len(p.Ports) == 0 {
				continue
			}
			filtered = append(filtered, p)
		}
	}

	if m.portsOnly {
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].Ports[0] < filtered[j].Ports[0]
		})
	}

	return filtered
}

// portsForSearch 是一个辅助函数，将端口号列表转换为一个用空格分隔的字符串，
// 以便用于模糊搜索。
func portsForSearch(ports []uint32) string {
	if len(ports) == 0 {
		return ""
	}
	return strings.Join(portsToStrings(ports), " ")
}

// portsToStrings 是一个辅助函数，将 `[]uint32` 类型的端口列表转换为 `[]string`。
func portsToStrings(ports []uint32) []string {
	parts := make([]string, len(ports))
	for i, port := range ports {
		parts[i] = strconv.FormatUint(uint64(port), 10)
	}
	return parts
}

// Start 是 TUI 模块的公共入口点。
// main.go 中的 main 函数会调用它来启动整个应用。
func Start(filter string) {
	// tea.NewProgram 创建一个新的 Bubble Tea 程序实例，
	// 并使用我们定义的 InitialModel 来初始化其状态。
	p := tea.NewProgram(InitialModel(filter))
	// p.Run() 启动事件循环，开始渲染UI并处理消息。
	// 这是一个阻塞调用，直到程序退出（例如用户按下 'q' 或 'ctrl+c'）。
	if _, err := p.Run(); err != nil {
		// 如果程序运行出错，则向标准错误输出打印错误信息并退出。
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
