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

// processesLoadedMsg 是一条消息，用于封装从后台 Goroutine 获取到的进程列表。
// 当 `process.GetProcesses` 函数成功执行完毕后，它会创建一个此类型的消息并发送给 `Update` 函数。
// 这条消息不仅包含了成功获取的进程列表，还可能包含一个 `warnings` 切片，
// 用于记录在获取某些进程信息时发生的非致命错误（例如，权限不足）。
type processesLoadedMsg struct {
	processes []*process.Item // 成功获取的进程列表。
	warnings  []error         // 获取过程中遇到的非致命错误。
}

// processDetailsMsg 是一条消息，用于携带从 `process.GetProcessDetails` 获取到的单个进程的详细信息。
// 它的类型是字符串，因为详细信息已经被格式化为适合直接显示的文本。
type processDetailsMsg string

// errMsg 是一条用于传递错误的专用消息。当任何命令（`tea.Cmd`）的执行过程中发生错误时，
// 它应该返回一个 `errMsg` 消息，以便 `Update` 函数可以捕获这个错误并更新模型状态，
// 最终在UI上向用户显示错误信息。
type errMsg struct{ err error }

// signalOKMsg 是一条反馈消息，表示向特定进程发送信号的操作已成功完成。
// 这是一种重要的设计模式：UI的更新（例如，将一个进程标记为“已杀死”）不应该在发送信号的命令被触发时立即发生，
// 而应该在这个命令成功执行并返回 `signalOKMsg` 之后才进行。这确保了UI状态与系统实际状态的一致性。
type signalOKMsg struct {
	pid    int            // 成功接收信号的进程的PID。
	status process.Status // 该进程的新状态（例如，Killed, Paused）。
}

// --- 应用状态模型 ---
// model 结构体是 Bubble Tea 应用的核心，它聚合了应用在任何时刻的所有状态。
// 它是UI的“单一数据源”（Single Source of Truth）。View 函数会根据这个 model 的数据来渲染界面，
// 而 Update 函数会根据接收到的消息来更新这个 model 的状态，并可能触发新的命令。
type model struct {
	// --- 核心数据 ---
	// processes 存储从系统中获取的原始、完整的进程列表。它作为一份不可变的缓存，
	// 所有的过滤和操作都基于这份数据，直到下一次刷新（`getProcesses` 命令）。
	processes []*process.Item
	// filtered 存储根据用户输入（搜索词）和视图模式（如 portsOnly）过滤后的进程列表。
	// 这是在主列表视图中实际向用户展示的数据。
	filtered []*process.Item

	// --- UI 状态 ---
	// cursor 表示当前用户界面上光标选中的项目在 `filtered` 列表中的索引。
	cursor int
	// textInput 是一个来自 `bubbles/textinput` 库的组件，用于管理搜索框的状态，
	// 包括输入内容、光标位置和是否获得焦点。
	textInput textinput.Model
	// warnings 存储从 `processesLoadedMsg` 中获取的非致命错误列表，用于在UI上提示用户。
	warnings []error

	// --- 视图模式与覆盖层 ---
	// err 用于存储在应用运行过程中可能发生的、需要向用户展示的错误。
	// 当 `err` 不为 `nil` 时，`View` 函数会渲染一个错误覆盖层。
	err error
	// showDetails 是一个布尔标志，用于控制是显示主进程列表还是显示单个进程的详细信息视图。
	showDetails bool
	// processDetails 存储从 `GetProcessDetails` 获取到的、准备在详情视图中显示的字符串。
	processDetails string
	// portsOnly 是一个布尔标志，当为 `true` 时，主列表只显示那些正在监听端口的进程。
	portsOnly bool
	// confirm 指向一个 `confirmPrompt` 结构体，当需要用户确认一个危险操作（如杀死进程）时，
	// 这个指针会被设置。当它不为 `nil` 时，`View` 函数会渲染一个确认对话框覆盖层。
	confirm *confirmPrompt
	// helpOpen 控制帮助菜单覆盖层是否显示。
	helpOpen bool

	// --- 依赖树 (T模式) 状态 ---
	// dep 聚合了所有与依赖树视图相关的状态，例如当前根进程、节点的展开/折叠状态等。
	// 将其封装在一个单独的结构体中可以使主 `model` 结构更清晰。
	dep depViewState
}

// InitialModel 创建并返回应用的初始状态模型。它在程序启动时被 `tea.NewProgram` 调用一次。
func InitialModel(filter string) model {
	ti := textinput.New()
	ti.Placeholder = "Search processes or ports"
	ti.CharLimit = 156
	ti.Width = 20
	ti.SetValue(filter)

	// 尝试从缓存文件加载上一次的进程列表。
	// 这里的 `_` 忽略了可能发生的错误（例如，首次运行或缓存文件损坏）。
	// 即使加载失败，`cached` 也会是一个空的 `[]*process.Item` 切片，程序可以正常继续。
	// 这种设计使得应用在等待实时数据时能立即显示一些（可能过时的）内容，提升了启动体验。
	cached, _ := process.Load()

	// 创建并初始化 model 结构体。
	m := model{
		textInput: ti,     // 设置文本输入框组件。
		processes: cached, // 使用加载的缓存数据作为初始的完整进程列表。
	}
	// 根据初始的过滤条件（可能来自命令行参数）对缓存数据进行一次过滤。
	m.filtered = m.filterProcesses(filter)
	return m
}

// --- 模糊搜索逻辑 ---
// 为了实现高效且用户友好的搜索功能，我们使用了 `sahilm/fuzzy` 库。
// 这个库需要一个实现了 `fuzzy.Source` 接口的数据源。
// `fuzzyProcessSource` 就是为我们的进程列表 `[]*process.Item` 实现这个接口的适配器。

// fuzzyProcessSource 是一个适配器结构体，它包装了我们的进程列表 `[]*process.Item`，
// 并实现了 `fuzzy.Source` 接口，使其能够被 `sahilm/fuzzy` 库使用。
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

// filterProcesses 是模型的一个核心方法，它根据当前的过滤字符串和视图模式（如 `portsOnly`）
// 来筛选 `m.processes` 中的进程，并返回一个新的 `filtered` 列表。
func (m *model) filterProcesses(filter string) []*process.Item {
	var result []*process.Item

	// --- 第一步：无过滤字符串的特殊处理 ---
	// 如果过滤字符串为空，逻辑很简单：遍历所有进程，应用 `portsOnly` 过滤器，并排除已杀死的进程。
	if filter == "" {
		for _, p := range m.processes {
			if p.Status == process.Killed {
				continue // 排除已标记为杀死的进程。
			}
			if m.portsOnly && len(p.Ports) == 0 {
				continue // 在 "ports-only" 模式下，排除没有监听端口的进程。
			}
			result = append(result, p)
		}
		// 在 "ports-only" 模式下，对结果按第一个端口号进行稳定排序。
		if m.portsOnly {
			sort.SliceStable(result, func(i, j int) bool {
				// 端口列表在采集时已保证有序，所以直接取第一个作为排序键。
				return result[i].Ports[0] < result[j].Ports[0]
			})
		}
		return result
	}

	// --- 第二步：使用模糊搜索 ---
	// 如果存在过滤字符串，则使用 `sahilm/fuzzy` 库进行高效的模糊匹配。
	// 1. 创建一个 `fuzzyProcessSource` 实例作为模糊搜索的数据源。
	source := fuzzyProcessSource{processes: m.processes}
	// 2. 调用 `fuzzy.FindFrom` 进行搜索，它会返回一个按匹配度排序的结果列表 `matches`。
	matches := fuzzy.FindFrom(filter, source)

	// 3. 根据匹配结果，从原始的 `m.processes` 列表中构建出过滤后的列表。
	for _, match := range matches {
		p := m.processes[match.Index]
		if p.Status == process.Killed {
			continue // 同样，排除已杀死的进程。
		}
		if m.portsOnly && len(p.Ports) == 0 {
			continue // 在 "ports-only" 模式下应用过滤器。
		}
		result = append(result, p)
	}

	// 在 "ports-only" 模式下，即使是模糊搜索的结果，也需要按端口号重新排序，
	// 因为模糊搜索的排序是基于匹配度，而不是端口号。
	if m.portsOnly {
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].Ports[0] < result[j].Ports[0]
		})
	}

	return result
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
