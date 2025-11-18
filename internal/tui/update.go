package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/w31r4/gokill/internal/process"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Dependency tree structures & confirm prompt ---

type depNodeState struct {
	expanded    bool
	page        int
	depthExtend int
}

type depLine struct {
	pid      int32
	parent   int32
	isMore   bool
	text     string
	depth    int
	isDeeper bool
}

type confirmPrompt struct {
	pid    int32
	name   string
	op     string // kill | pause | resume
	sig    syscall.Signal
	status process.Status
}

// Init 是 Bubble Tea 接口的一部分，在程序首次运行时调用。
// 它负责执行一些初始化的命令（`tea.Cmd`）。
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
		// Help overlay handling
		if m.helpOpen {
			switch msg.String() {
			case "?", "esc":
				m.helpOpen = false
				return m, nil
			case "ctrl+c", "q":
				return m, tea.Quit
			}
			return m, nil
		}
		// Confirm overlay handling takes precedence (except errors already handled above).
		if m.confirm != nil {
			switch msg.String() {
			case "y", "enter":
				op := *m.confirm
				m.confirm = nil
				return m, sendSignalWithStatus(int(op.pid), op.sig, op.status)
			case "n", "esc":
				m.confirm = nil
				return m, nil
			case "ctrl+c", "q":
				return m, tea.Quit
			}
			return m, nil
		}

		// Dependency mode (T) takes precedence over other interactive states except errors and confirm.
		if m.depMode {
			// When in T-mode and search is focused, update input first.
			if m.textInput.Focused() {
				switch msg.String() {
				case "enter", "esc":
					m.textInput.Blur()
				}
				m.textInput, cmd = m.textInput.Update(msg)
				// keep cursor clamped to filtered lines
				if c := len(applyDepFilters(m, buildDepLines(m))); m.depCursor >= c {
					if c > 0 {
						m.depCursor = c - 1
					} else {
						m.depCursor = 0
					}
				}
				return m, cmd
			}
			switch msg.String() {
			case "esc":
				m.depMode = false
				m.depExpanded = nil
				m.depCursor = 0
				return m, nil
			case "ctrl+c", "q":
				return m, tea.Quit
			case "ctrl+r":
				return m, getProcesses
			case "/":
				m.textInput.Focus()
				return m, nil
			case "?":
				m.helpOpen = true
				return m, nil
			case "S":
				m.depAliveOnly = !m.depAliveOnly
				if c := len(applyDepFilters(m, buildDepLines(m))); m.depCursor >= c {
					if c > 0 {
						m.depCursor = c - 1
					} else {
						m.depCursor = 0
					}
				}
				return m, nil
			case "L":
				m.depPortsOnly = !m.depPortsOnly
				if c := len(applyDepFilters(m, buildDepLines(m))); m.depCursor >= c {
					if c > 0 {
						m.depCursor = c - 1
					} else {
						m.depCursor = 0
					}
				}
				return m, nil
			case "up", "k":
				if m.depCursor > 0 {
					m.depCursor--
				}
				return m, nil
			case "down", "j":
				if m.depCursor < len(buildDepLines(m))-1 {
					m.depCursor++
				}
				return m, nil
			case "left", "h":
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil
				}
				ln := lines[m.depCursor]
				if ln.isMore {
					// collapse parent and reset deeper/page state
					st := m.depExpanded[ln.parent]
					st.expanded = false
					st.page = 1
					st.depthExtend = 0
					m.depExpanded[ln.parent] = st
				} else if ln.pid != 0 {
					st := m.depExpanded[ln.pid]
					st.expanded = false
					if st.page == 0 {
						st.page = 1
					}
					m.depExpanded[ln.pid] = st
				}
				if m.depCursor >= len(buildDepLines(m)) {
					if c := len(buildDepLines(m)); c > 0 {
						m.depCursor = c - 1
					} else {
						m.depCursor = 0
					}
				}
				return m, nil
			case "right", "l":
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil
				}
				ln := lines[m.depCursor]
				if ln.isMore {
					st := m.depExpanded[ln.parent]
					if ln.isDeeper {
						st.depthExtend++
						st.expanded = true
					} else {
						if st.page == 0 {
							st.page = 1
						}
						st.page++
						st.expanded = true
					}
					m.depExpanded[ln.parent] = st
				} else if ln.pid != 0 {
					st := m.depExpanded[ln.pid]
					if st.page == 0 {
						st.page = 1
					}
					st.expanded = true
					m.depExpanded[ln.pid] = st
				}
				if m.depCursor >= len(buildDepLines(m)) {
					if c := len(buildDepLines(m)); c > 0 {
						m.depCursor = c - 1
					} else {
						m.depCursor = 0
					}
				}
				return m, nil
			case " ":
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil
				}
				ln := lines[m.depCursor]
				if ln.isMore {
					st := m.depExpanded[ln.parent]
					if ln.isDeeper {
						st.depthExtend++
						st.expanded = true
					} else {
						if st.page == 0 {
							st.page = 1
						}
						st.page++
						st.expanded = true
					}
					m.depExpanded[ln.parent] = st
				} else if ln.pid != 0 {
					st := m.depExpanded[ln.pid]
					if st.page == 0 {
						st.page = 1
					}
					st.expanded = !st.expanded
					if st.page == 0 {
						st.page = 1
					}
					m.depExpanded[ln.pid] = st
				}
				if m.depCursor >= len(buildDepLines(m)) {
					if c := len(buildDepLines(m)); c > 0 {
						m.depCursor = c - 1
					} else {
						m.depCursor = 0
					}
				}
				return m, nil
			case "enter", "o":
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil
				}
				ln := lines[m.depCursor]
				if ln.pid != 0 {
					m.depRootPID = ln.pid
					if m.depExpanded == nil {
						m.depExpanded = make(map[int32]depNodeState)
					}
					m.depExpanded = map[int32]depNodeState{ln.pid: {expanded: true, page: 1}}
					m.depCursor = 0
				}
				return m, nil
			case "u":
				if root := m.findProcess(m.depRootPID); root != nil {
					if parent := m.findProcess(root.PPid); parent != nil {
						m.depRootPID = parent.Pid
						if m.depExpanded == nil {
							m.depExpanded = make(map[int32]depNodeState)
						}
						m.depExpanded = map[int32]depNodeState{parent.Pid: {expanded: true, page: 1}}
						m.depCursor = 0
					}
				}
				return m, nil
			case "a":
				m.depShowAncestors = !m.depShowAncestors
				return m, nil
			case "i":
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil
				}
				ln := lines[m.depCursor]
				if ln.pid != 0 {
					m.showDetails = true
					m.processDetails = ""
					return m, getProcessDetails(int(ln.pid))
				}
				return m, nil
			case "x":
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil
				}
				ln := lines[m.depCursor]
				if ln.pid != 0 {
					if it := m.findProcess(ln.pid); it != nil {
						m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "kill", sig: syscall.SIGTERM, status: process.Killed}
					}
				}
				return m, nil
			case "p":
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil
				}
				ln := lines[m.depCursor]
				if ln.pid != 0 {
					if it := m.findProcess(ln.pid); it != nil {
						m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "pause", sig: syscall.SIGSTOP, status: process.Paused}
					}
				}
				return m, nil
			case "r":
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil
				}
				ln := lines[m.depCursor]
				if ln.pid != 0 {
					if it := m.findProcess(ln.pid); it != nil && it.Status == process.Paused {
						m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "resume", sig: syscall.SIGCONT, status: process.Alive}
					}
				}
				return m, nil
			}
		}
		if m.err != nil {
			switch msg.String() {
			case "esc":
				m.err = nil
			case "ctrl+c", "q":
				return m, tea.Quit
			}
			return m, nil
		}

		if m.showDetails {
			switch msg.String() {
			case "esc":
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
		case "?":
			m.helpOpen = true
			return m, nil
		case "/":
			m.textInput.Focus()
			return m, nil
		case "esc":
			// ESC 退出模式：此处用于退出 ports-only 视图
			if m.portsOnly {
				m.portsOnly = false
				m.filtered = m.filterProcesses(m.textInput.Value())
				return m, nil
			}
		case "P":
			// 进入“仅显示占用端口的进程”模式；退出由 ESC 统一处理
			if !m.portsOnly {
				m.portsOnly = true
				m.filtered = m.filterProcesses(m.textInput.Value())
			}
			return m, nil
		case "T":
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				m.depMode = true
				m.depRootPID = p.Pid
				if m.depExpanded == nil {
					m.depExpanded = make(map[int32]depNodeState)
				}
				m.depExpanded[p.Pid] = depNodeState{expanded: true, page: 1}
				m.depCursor = 0
			}
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
				m.processDetails = ""
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

// buildDepLines 将当前依赖树按展开/分页状态扁平化为行。
func buildDepLines(m model) []depLine {
	root := m.findProcess(m.depRootPID)
	if root == nil {
		return nil
	}
	childrenMap := m.buildChildrenMap()

	var lines []depLine
	// 根行（深度0）
	lines = append(lines, depLine{pid: root.Pid, parent: 0, isMore: false, text: fmt.Sprintf("%s (%d)", root.Executable, root.Pid), depth: 0})

	// 递归子节点
	var walk func(pid int32, prefix string, depth int)
	walk = func(pid int32, prefix string, depth int) {
		kids := childrenMap[pid]
		if len(kids) == 0 {
			return
		}

		// 排序稳定
		sort.Slice(kids, func(i, j int) bool {
			if kids[i].Executable == kids[j].Executable {
				return kids[i].Pid < kids[j].Pid
			}
			return kids[i].Executable < kids[j].Executable
		})

		// 展开状态与分页
		st := m.depExpanded[pid]
		if !st.expanded && pid != m.depRootPID {
			return
		}
		page := st.page
		if page <= 0 {
			page = 1
		}
		limit := dependencyTreeChildLimit * page
		show := len(kids)
		if show > limit {
			show = limit
		}

		for i := 0; i < show; i++ {
			child := kids[i]
			last := (i == show-1) && (show == len(kids))
			connector := branchSymbol(last)
			line := fmt.Sprintf("%s%s %s (%d)", prefix, connector, child.Executable, child.Pid)
			lines = append(lines, depLine{pid: child.Pid, parent: pid, text: line, depth: depth + 1})

			nextPrefix := prefix
			if last {
				nextPrefix += "   "
			} else {
				nextPrefix += "│  "
			}

			// allow per-parent deeper expansion beyond global depth
			allowed := dependencyTreeDepth - 1 + m.depExpanded[pid].depthExtend
			if depth < allowed {
				walk(child.Pid, nextPrefix, depth+1)
			} else if len(childrenMap[child.Pid]) > 0 {
				// depth limit reached; add interactive deeper placeholder
				moreLine := fmt.Sprintf("%s└─ … (deeper)", nextPrefix)
				lines = append(lines, depLine{pid: 0, parent: child.Pid, isMore: true, isDeeper: true, text: moreLine, depth: depth + 1})
			}
		}

		if show < len(kids) {
			// 还有更多同级子项，提供分页提示行
			more := len(kids) - show
			connector := branchSymbol(true)
			moreLine := fmt.Sprintf("%s%s … (%d more)", prefix, connector, more)
			lines = append(lines, depLine{pid: 0, parent: pid, isMore: true, isDeeper: false, text: moreLine, depth: depth})
		}
	}

	// 根默认展开
	if st, ok := m.depExpanded[root.Pid]; !ok || !st.expanded {
		m.depExpanded[root.Pid] = depNodeState{expanded: true, page: 1}
	}
	walk(root.Pid, "", 0)
	return lines
}

// applyDepFilters 根据 T 模式的筛选条件过滤行：文本过滤、仅存活、仅监听端口。
func applyDepFilters(m model, lines []depLine) []depLine {
	if len(lines) == 0 {
		return lines
	}

	term := strings.TrimSpace(m.textInput.Value())
	hasTerm := term != ""
	var out []depLine
	for _, ln := range lines {
		// never drop paging/ellipsis lines完全, but they are not actionable
		if ln.pid == 0 {
			if hasTerm {
				// skip ellipsis on filter to reduce noise
				continue
			}
			out = append(out, ln)
			continue
		}
		it := m.findProcess(ln.pid)
		if it == nil {
			continue
		}
		if m.depAliveOnly && it.Status != process.Alive {
			continue
		}
		if m.depPortsOnly && len(it.Ports) == 0 {
			continue
		}
		if hasTerm {
			// case-insensitive substring on text and pid match
			if !strings.Contains(strings.ToLower(ln.text), strings.ToLower(term)) {
				if termPid, err := strconv.Atoi(term); err == nil {
					if int32(termPid) != it.Pid {
						continue
					}
				} else {
					continue
				}
			}
		}
		out = append(out, ln)
	}
	return out
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

// branchSymbol returns the tree drawing character for normal vs last child.
func branchSymbol(last bool) string {
	if last {
		return "└─"
	}
	return "├─"
}

