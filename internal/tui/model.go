package tui

import (
    "fmt"
    "os"
    "sort"
    "strconv"
    "strings"
    "syscall"

	"github.com/w31r4/gokill/internal/process"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// --- Bubble Tea 消息定义 ---
// Bubble Tea 框架通过消息（Message, `tea.Msg`）来驱动应用状态的更新。
// 消息可以是任何类型，通常是结构体或自定义类型。它们由命令（`tea.Cmd`）产生，或由框架自身因用户输入（如按键）而生成。

// processesMsg 是一个自定义消息类型，用于在获取到进程列表后，将其传递给 Update 方法。
// 它本质上是一个 `process.Item` 切片的别名，携带了所有进程的信息。
// processesLoadedMsg is a custom message that wraps the results of fetching
// processes, including the list of successfully retrieved items and any
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

// Init 是 Bubble Tea 接口的一部分，在程序首次运行时调用。
// 它负责执行一些初始化的命令（`tea.Cmd`）。命令是执行 I/O 操作（如网络请求、文件读写、定时器等）的函数，
// 执行完毕后会返回一个消息（`tea.Msg`）给 Update 方法。
func (m model) Init() tea.Cmd {
	// tea.Batch 用于将多个命令合并成一个。
	// 这里的 `m.textInput.Focus()` 是一个命令，它使文本输入框获得焦点。
	// `getProcesses` 是我们定义的另一个命令，用于异步获取进程列表。
	// 这两个命令会并发执行。
	return tea.Batch(m.textInput.Focus(), getProcesses)
}

// getProcesses 是一个命令（`tea.Cmd`），它封装了获取进程列表的逻辑。
// 命令本质上是一个函数，其返回值必须是 `tea.Msg`。
// Bubble Tea 运行时会负责在另一个 Goroutine 中执行这个函数，从而避免阻塞UI主循环。
func getProcesses() tea.Msg {
	procs, warnings, err := process.GetProcesses()
	if err != nil {
		return errMsg{err}
	}
	return processesLoadedMsg{processes: procs, warnings: warnings}
}

// getProcessDetails 返回一个获取特定进程详细信息的命令。
// 这是一个典型的命令工厂函数，它接收参数（pid），并返回一个闭包作为实际的 `tea.Cmd`。
func getProcessDetails(pid int) tea.Cmd {
	return func() tea.Msg {
		// 在这个 Goroutine 中执行耗时的操作。
		details, err := process.GetProcessDetails(pid)
		if err != nil {
			// 如果出错，返回一个 errMsg 消息。
			return errMsg{err}
		}
		// 成功则返回一个 processDetailsMsg 消息。
		return processDetailsMsg(details)
	}
}

// Update 是 Bubble Tea 架构的核心函数，负责处理所有消息并更新应用状态（model）。
// 它接收一个消息 `tea.Msg` 作为参数，返回更新后的模型 `tea.Model` 和一个可选的、需要执行的新命令 `tea.Cmd`。
// 这是整个应用逻辑的“状态机”。
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	// 1. 处理异步获取到的进程列表
	// 1. Handle the arrival of the process list and any associated warnings.
    case processesLoadedMsg:
        m.processes = msg.processes                         // Update the full process list cache.
        m.warnings = msg.warnings                           // Store any warnings.
        m.filtered = m.filterProcesses(m.textInput.Value()) // Re-filter based on the current search term.
        // Return a command to asynchronously save the new process list to the cache file.
        return m, func() tea.Msg {
            _ = process.Save(m.processes)
            return nil // This command doesn't need to trigger any subsequent updates.
        }

	// 2. 处理异步获取到的进程详情
	case processDetailsMsg:
		m.processDetails = string(msg) // 更新模型中的详情字符串
		return m, nil                  // 不需要执行新的命令

	// 3. 处理错误消息
    case errMsg:
        // 我们只显示错误信息，但有一种特殊情况需要忽略：
        // 当我们尝试操作一个已经结束的进程时，会收到 "process already finished" 错误，
        // 这在并发场景下是正常现象，直接忽略即可，避免不必要的信息干扰用户。
        if !strings.Contains(msg.err.Error(), "process already finished") {
            m.err = msg.err
        }
        return m, nil

    // 处理信号成功消息：在成功后才更新 UI 状态，避免失败导致的 UI 错乱。
    case signalOKMsg:
        for _, it := range m.processes {
            if int(it.Pid) == msg.pid {
                it.Status = msg.status
                break
            }
        }
        // 由于 filtered 指向同一批指针，直接返回并让 View 重新渲染即可。
        return m, nil

	// 4. 处理用户按键输入
	case tea.KeyMsg:
		if m.showDetails {
			switch msg.String() {
			case "q", "esc", "i":
				m.showDetails = false
				m.processDetails = "" // Clear details
			}
			return m, nil
		}

		if m.textInput.Focused() {
			switch msg.String() {
			case "enter", "esc":
				m.textInput.Blur()
			}
			m.textInput, cmd = m.textInput.Update(msg)
			m.filtered = m.filterProcesses(m.textInput.Value())
			return m, cmd
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "ctrl+r":
			return m, getProcesses
		case "/":
			m.textInput.Focus()
			return m, nil
		case "P":
			// 切换“仅显示占用端口的进程”模式
			m.portsOnly = !m.portsOnly
			m.filtered = m.filterProcesses(m.textInput.Value())
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
        case "enter":
            if len(m.filtered) > 0 {
                p := m.filtered[m.cursor]
                return m, sendSignalWithStatus(int(p.Pid), syscall.SIGTERM, process.Killed)
            }
        case "p":
            if len(m.filtered) > 0 {
                p := m.filtered[m.cursor]
                return m, sendSignalWithStatus(int(p.Pid), syscall.SIGSTOP, process.Paused)
            }
        case "r":
            if len(m.filtered) > 0 {
                p := m.filtered[m.cursor]
                if p.Status == process.Paused {
                    return m, sendSignalWithStatus(int(p.Pid), syscall.SIGCONT, process.Alive)
                }
            }
		case "i":
			if len(m.filtered) > 0 {
				m.showDetails = true
				p := m.filtered[m.cursor]
				return m, getProcessDetails(int(p.Pid))
			}
		}
	}

	var filterCmd tea.Cmd
	m.textInput, filterCmd = m.textInput.Update(msg)

	// Filter the processes
	m.filtered = m.filterProcesses(m.textInput.Value())

	// Clamp cursor to the new filtered list
	if m.cursor >= len(m.filtered) {
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		} else {
			m.cursor = 0
		}
	}

	return m, tea.Batch(cmd, filterCmd)
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
// 为了让用户可以同时通过进程名、PID或端口号进行搜索，我们将这几项信息拼接成一个单一的字符串。
func (s fuzzyProcessSource) String(i int) string {
	p := s.processes[i]
	if ports := portsForSearch(p.Ports); ports != "" {
		return fmt.Sprintf("%s %d %s", p.Executable, p.Pid, ports)
	}
	return fmt.Sprintf("%s %d", p.Executable, p.Pid)
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

// sendSignal 是一个命令工厂函数，用于创建一个向指定PID进程发送信号的命令。
func sendSignal(pid int, sig syscall.Signal) tea.Cmd {
    return func() tea.Msg {
        // 在 Goroutine 中执行实际的信号发送操作。
        if err := process.SendSignal(pid, sig); err != nil {
            // 如果失败，返回一个错误消息。
            return errMsg{err}
        }
        // 成功则返回 nil，表示此命令不需要触发任何状态更新。
        return nil
    }
}

// sendSignalWithStatus 仅在信号发送成功后回传一条消息，用于更新 UI 中的进程状态。
func sendSignalWithStatus(pid int, sig syscall.Signal, status process.Status) tea.Cmd {
    return func() tea.Msg {
        if err := process.SendSignal(pid, sig); err != nil {
            return errMsg{err}
        }
        return signalOKMsg{pid: pid, status: status}
    }
}

// --- UI 样式定义 ---
// 使用 `charmbracelet/lipgloss` 库来定义TUI的样式。
// 这种方式使得样式的管理和复用变得非常方便。
var (
    // docStyle 是整个应用的基础样式，设置了外边距。
    // 收紧整体上下外边距让界面更紧凑。
    docStyle = lipgloss.NewStyle().Margin(0, 1)
	// selectedStyle 是当前光标选中行的样式，设置了背景色和前景色。
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255"))
	// faintStyle 用于渲染次要信息（如帮助文本），使其颜色变淡。
	faintStyle = lipgloss.NewStyle().Faint(true)
	// killingStyle 是标记为“已杀死”的进程的样式，使用了删除线和红色。
	killingStyle = lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("9"))
	// pausedStyle 是标记为“已暂停”的进程的样式，使用了黄色。
	pausedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	// listeningStyle 是监听端口的进程的样式，同样使用黄色以保持一致性。
	listeningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
    // paneStyle 是左右两个面板的基础样式，定义了圆角边框和内边距。
    paneStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
    // processPaneStyle 是左侧进程列表面板的样式，继承自 paneStyle 并设置了宽度和边框颜色。
    processPaneStyle = paneStyle.Copy().Width(60).BorderForeground(lipgloss.Color("62"))
    // portPaneStyle 是右侧端口列表面板的样式，继承自 paneStyle 并设置了宽度和边框颜色。
    // 取消固定高度与居中对齐，避免出现大量垂直空白；减小宽度使其更紧凑。
    portPaneStyle = paneStyle.Copy().Width(16).BorderForeground(lipgloss.Color("220")).Align(lipgloss.Left)
)

// 定义了进程列表视口（Viewport）的高度，即一次显示多少行。
const (
	viewHeight = 7
)

// View 函数根据当前的应用状态（model）生成一个字符串，用于在终端上显示。
// Bubble Tea 的运行时会不断调用这个函数来重绘界面。
// 这个函数应该是“纯”的，即不应有任何副作用，只负责根据 `m` 的数据渲染视图。
func (m model) View() string {
	// 如果模型中存在错误，则只显示错误信息。
	if m.err != nil {
		return fmt.Sprintf("\nError: %v\n\n", m.err)
	}

	if len(m.processes) == 0 {
		return "Loading processes..."
	}

	if m.showDetails {
		return m.renderDetailsView()
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	if len(m.filtered) == 0 {
		noResults := "  No results..."
		return docStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, noResults, footer))
	}

	processPane := m.renderProcessPane()
	portPane := m.renderPortPane()

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, processPane, portPane)

	return docStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, mainContent, footer))
}

// --- 视图渲染函数 ---
// `View` 函数会调用这些辅助函数来构建UI的不同部分。

// renderDetailsView 负责渲染单个进程的详细信息视图。
func (m model) renderDetailsView() string {
	var b strings.Builder
	fmt.Fprintln(&b, "Process Details")
	if m.processDetails == "" {
		// 如果详情还在加载中，显示 "Loading..."
		fmt.Fprintln(&b, "\n  Loading...")
	} else {
		// 加载完成后，显示获取到的详情字符串。
		fmt.Fprintln(&b, "")
		fmt.Fprint(&b, m.processDetails)
	}
	// 在底部添加帮助信息。
	help := faintStyle.Render("\n\n q/esc/i: back to list")
	fmt.Fprint(&b, help)
	// 应用整体的文档样式。
	return docStyle.Render(b.String())
}

// renderHeader 负责渲染应用的头部区域，主要包括搜索框和进程计数。
func (m model) renderHeader() string {
    var warnings string
    if len(m.warnings) > 0 {
        warnings = faintStyle.Render(fmt.Sprintf(" (%d warnings)", len(m.warnings)))
    }
    // Display "(filtered_count/total_count)"
    count := fmt.Sprintf("(%d/%d)", len(m.filtered), len(m.processes))
    mode := ""
    if m.portsOnly {
        mode = faintStyle.Render(" [ports-only]")
    }
    // Join title, count, warnings, mode and the text input view.
    return fmt.Sprintf("Search processes/ports %s%s%s: %s", faintStyle.Render(count), warnings, mode, m.textInput.View())
}

// renderFooter 负责渲染应用的底部区域，主要是根据当前状态显示不同的帮助信息。
func (m model) renderFooter() string {
    var help strings.Builder
    if m.textInput.Focused() {
        // 当搜索框激活时，显示退出搜索的提示。
        help.WriteString(faintStyle.Render(" enter/esc to exit search"))
    } else {
        // 否则，显示主界面的快捷键帮助。
        help.WriteString(faintStyle.Render(" /: search • i: info • P: ports-only • ctrl+r: refresh • r: resume • p: pause • enter: kill • q: quit"))
    }
    return help.String()
}

// renderProcessPane 负责渲染左侧的进程列表面板。
func (m model) renderProcessPane() string {
	var b strings.Builder

	// --- 视口（Viewport）计算 ---
	// 为了只显示屏幕可见区域的进程，而不是一次性渲染全部（可能有数千个），
	// 我们需要计算一个“视口”，使其始终以光标 `m.cursor` 为中心。
	start := m.cursor - viewHeight/2
	if start < 0 {
		start = 0 // 确保起始索引不小于0
	}
	end := start + viewHeight
	if end > len(m.filtered) {
		end = len(m.filtered) // 确保结束索引不超过列表长度
		// 当光标接近列表末尾时，重新计算 start，以保持视口大小不变。
		start = end - viewHeight
		if start < 0 {
			start = 0
		}
	}

	// --- 渲染视口内的每一行 ---
	for i := start; i < end; i++ {
		p := m.filtered[i]
		status := " "
		switch p.Status {
		case process.Killed:
			status = "K"
		case process.Paused:
			status = "P"
		}
		line := fmt.Sprintf("[%s] %-20s %-8s %-10s %d", status, p.Executable, p.StartTime, p.User, p.Pid)

		switch p.Status {
		case process.Killed:
			line = killingStyle.Render(line)
		case process.Paused:
			line = pausedStyle.Render(line)
		default:
			if len(p.Ports) > 0 {
				line = listeningStyle.Render(line)
			}
		}

		if i == m.cursor {
			fmt.Fprintln(&b, selectedStyle.Render("❯ "+line))
		} else {
			fmt.Fprintln(&b, "  "+faintStyle.Render(line))
		}
	}

    // 去掉末尾多余的换行，避免左侧列表底部出现空行。
    return processPaneStyle.Render(strings.TrimRight(b.String(), "\n"))
}

// renderPortPane 负责渲染右侧的端口信息面板。
func (m model) renderPortPane() string {
    var b strings.Builder
    fmt.Fprintln(&b, "Listening Ports")

	// 如果没有进程或光标无效，则显示 n/a。
    if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
        fmt.Fprintln(&b, faintStyle.Render("(n/a)"))
        return portPaneStyle.Render(strings.TrimRight(b.String(), "\n"))
    }

	// 获取当前光标选中的进程。
	p := m.filtered[m.cursor]
    if len(p.Ports) == 0 {
        // 如果该进程没有监听任何端口，则显示 (none)。
        fmt.Fprintln(&b, faintStyle.Render("(none)"))
    } else {
        // 否则，逐行显示所有监听的端口号。
        for _, port := range p.Ports {
            fmt.Fprintln(&b, fmt.Sprintf("%d", port))
        }
    }

	// 应用端口面板的样式。
    return portPaneStyle.Render(strings.TrimRight(b.String(), "\n"))
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
