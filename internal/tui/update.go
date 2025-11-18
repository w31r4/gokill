package tui

import (
	"strings"
	"syscall"

	"github.com/w31r4/gokill/internal/process"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Confirm prompt state ---

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
		newModel, keyCmd, handled := m.updateKeyMsg(msg)
		if handled {
			return newModel, keyCmd
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

	return m, filterCmd
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

// updateKeyMsg 根据当前界面状态分发按键处理。
// 它返回更新后的 model、可选命令以及一个布尔值表示是否已完全处理该按键。
func (m model) updateKeyMsg(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	// Help overlay handling
	if m.helpOpen {
		newModel, cmd := m.updateHelpKey(msg)
		return newModel, cmd, true
	}

	// Confirm overlay handling takes precedence (except errors already handled above).
	if m.confirm != nil {
		newModel, cmd := m.updateConfirmKey(msg)
		return newModel, cmd, true
	}

	// Dependency mode (T) takes precedence over other states except help/confirm.
	if m.dep.mode {
		newModel, cmd, handled := m.updateDepModeKey(msg)
		if handled {
			return newModel, cmd, true
		}
		m = newModel
	}

	// Error overlay
	if m.err != nil {
		newModel, cmd := m.updateErrorKey(msg)
		return newModel, cmd, true
	}

	// Details view
	if m.showDetails {
		newModel, cmd := m.updateDetailsKey(msg)
		return newModel, cmd, true
	}

	// Search-focused handling in main list mode.
	if m.textInput.Focused() {
		newModel, cmd := m.updateSearchKey(msg)
		return newModel, cmd, true
	}

	// Fallback to main list key handling.
	return m.updateMainListKey(msg)
}

// updateHelpKey handles key events when the help overlay is open.
func (m model) updateHelpKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyRunes:
		if len(msg.Runes) == 1 && msg.Runes[0] == '?' {
			m.helpOpen = false
			return m, nil
		}
	case tea.KeyEsc:
		m.helpOpen = false
		return m, nil
	case tea.KeyCtrlC:
		return m, tea.Quit
	}
	// 'q' to quit when help is open.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
		return m, tea.Quit
	}
	return m, nil
}

// updateConfirmKey handles key events when the confirm overlay is active.
func (m model) updateConfirmKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		op := *m.confirm
		m.confirm = nil
		return m, sendSignalWithStatus(int(op.pid), op.sig, op.status)
	case tea.KeyEsc:
		m.confirm = nil
		return m, nil
	case tea.KeyCtrlC:
		return m, tea.Quit
	}
	// rune-based confirmations/cancels.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'y':
			op := *m.confirm
			m.confirm = nil
			return m, sendSignalWithStatus(int(op.pid), op.sig, op.status)
		case 'n':
			m.confirm = nil
			return m, nil
		case 'q':
			return m, tea.Quit
		}
	}
	return m, nil
}

// updateDepModeKey handles key events in dependency-tree (T) mode.
// It returns handled=false for keys that should fall back to main list handling.
func (m model) updateDepModeKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	var cmd tea.Cmd

	// When in T-mode and search is focused, update input first.
	if m.textInput.Focused() {
		switch msg.Type {
		case tea.KeyEnter, tea.KeyEsc:
			m.textInput.Blur()
		}
		m.textInput, cmd = m.textInput.Update(msg)
		// keep cursor clamped to filtered lines
		if c := len(applyDepFilters(m, buildDepLines(m))); m.dep.cursor >= c {
			if c > 0 {
				m.dep.cursor = c - 1
			} else {
				m.dep.cursor = 0
			}
		}
		return m, cmd, true
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.dep.mode = false
		m.dep.expanded = nil
		m.dep.cursor = 0
		return m, nil, true
	case tea.KeyCtrlC:
		return m, tea.Quit, true
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case '/':
				m.textInput.Focus()
				return m, nil, true
			case '?':
				m.helpOpen = true
				return m, nil, true
			case 'S':
				m.dep.aliveOnly = !m.dep.aliveOnly
				if c := len(applyDepFilters(m, buildDepLines(m))); m.dep.cursor >= c {
					if c > 0 {
						m.dep.cursor = c - 1
					} else {
						m.dep.cursor = 0
					}
				}
				return m, nil, true
			case 'L':
				m.dep.portsOnly = !m.dep.portsOnly
				if c := len(applyDepFilters(m, buildDepLines(m))); m.dep.cursor >= c {
					if c > 0 {
						m.dep.cursor = c - 1
					} else {
						m.dep.cursor = 0
					}
				}
				return m, nil, true
			case 'u':
				if root := m.findProcess(m.dep.rootPID); root != nil {
					if parent := m.findProcess(root.PPid); parent != nil {
						m.dep.rootPID = parent.Pid
						if m.dep.expanded == nil {
							m.dep.expanded = make(map[int32]depNodeState)
						}
						m.dep.expanded = map[int32]depNodeState{parent.Pid: {expanded: true, page: 1}}
						m.dep.cursor = 0
					}
				}
				return m, nil, true
			case 'a':
				m.dep.showAncestors = !m.dep.showAncestors
				return m, nil, true
			case 'i':
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil, true
				}
				ln := lines[m.dep.cursor]
				if ln.pid != 0 {
					m.showDetails = true
					m.processDetails = ""
					return m, getProcessDetails(int(ln.pid)), true
				}
				return m, nil, true
			case 'x':
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil, true
				}
				ln := lines[m.dep.cursor]
				if ln.pid != 0 {
					if it := m.findProcess(ln.pid); it != nil {
						m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "kill", sig: syscall.SIGTERM, status: process.Killed}
					}
				}
				return m, nil, true
			case 'p':
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil, true
				}
				ln := lines[m.dep.cursor]
				if ln.pid != 0 {
					if it := m.findProcess(ln.pid); it != nil {
						m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "pause", sig: syscall.SIGSTOP, status: process.Paused}
					}
				}
				return m, nil, true
			case 'r':
				lines := buildDepLines(m)
				if len(lines) == 0 {
					return m, nil, true
				}
				ln := lines[m.dep.cursor]
				if ln.pid != 0 {
					if it := m.findProcess(ln.pid); it != nil && it.Status == process.Paused {
						m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "resume", sig: syscall.SIGCONT, status: process.Alive}
					}
				}
				return m, nil, true
			}
		}
	case tea.KeyCtrlR:
		return m, getProcesses, true
	case tea.KeyUp:
		if m.dep.cursor > 0 {
			m.dep.cursor--
		}
		return m, nil, true
	case tea.KeyDown:
		if m.dep.cursor < len(buildDepLines(m))-1 {
			m.dep.cursor++
		}
		return m, nil, true
	case tea.KeyLeft:
		lines := buildDepLines(m)
		if len(lines) == 0 {
			return m, nil, true
		}
		ln := lines[m.dep.cursor]
		if ln.isMore {
			// collapse parent and reset deeper/page state
			st := m.dep.expanded[ln.parent]
			st.expanded = false
			st.page = 1
			st.depthExtend = 0
			m.dep.expanded[ln.parent] = st
		} else if ln.pid != 0 {
			st := m.dep.expanded[ln.pid]
			st.expanded = false
			if st.page == 0 {
				st.page = 1
			}
			m.dep.expanded[ln.pid] = st
		}
		if m.dep.cursor >= len(buildDepLines(m)) {
			if c := len(buildDepLines(m)); c > 0 {
				m.dep.cursor = c - 1
			} else {
				m.dep.cursor = 0
			}
		}
		return m, nil, true
	case tea.KeyRight:
		lines := buildDepLines(m)
		if len(lines) == 0 {
			return m, nil, true
		}
		ln := lines[m.dep.cursor]
		if ln.isMore {
			st := m.dep.expanded[ln.parent]
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
			m.dep.expanded[ln.parent] = st
		} else if ln.pid != 0 {
			st := m.dep.expanded[ln.pid]
			if st.page == 0 {
				st.page = 1
			}
			st.expanded = true
			m.dep.expanded[ln.pid] = st
		}
		if m.dep.cursor >= len(buildDepLines(m)) {
			if c := len(buildDepLines(m)); c > 0 {
				m.dep.cursor = c - 1
			} else {
				m.dep.cursor = 0
			}
		}
		return m, nil, true
	case tea.KeySpace:
		lines := buildDepLines(m)
		if len(lines) == 0 {
			return m, nil, true
		}
		ln := lines[m.dep.cursor]
		if ln.isMore {
			st := m.dep.expanded[ln.parent]
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
			m.dep.expanded[ln.parent] = st
		} else if ln.pid != 0 {
			st := m.dep.expanded[ln.pid]
			if st.page == 0 {
				st.page = 1
			}
			st.expanded = !st.expanded
			if st.page == 0 {
				st.page = 1
			}
			m.dep.expanded[ln.pid] = st
		}
		if m.dep.cursor >= len(buildDepLines(m)) {
			if c := len(buildDepLines(m)); c > 0 {
				m.dep.cursor = c - 1
			} else {
				m.dep.cursor = 0
			}
		}
		return m, nil, true
	case tea.KeyEnter:
		lines := buildDepLines(m)
		if len(lines) == 0 {
			return m, nil, true
		}
		ln := lines[m.dep.cursor]
		if ln.pid != 0 {
			m.dep.rootPID = ln.pid
			if m.dep.expanded == nil {
				m.dep.expanded = make(map[int32]depNodeState)
			}
			m.dep.expanded = map[int32]depNodeState{ln.pid: {expanded: true, page: 1}}
			m.dep.cursor = 0
		}
		return m, nil, true
	default:
		// fall through to generic fallback below
	}

	// Unhandled keys in depMode should fall back to main list handling.
	return m, nil, false
}

// updateErrorKey handles key events when an error overlay is shown.
func (m model) updateErrorKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.err = nil
		return m, nil
	case tea.KeyCtrlC:
		return m, tea.Quit
	}
	// allow 'q' to quit from error overlay.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
		return m, tea.Quit
	}
	return m, nil
}

// updateDetailsKey handles key events in the details view.
func (m model) updateDetailsKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.showDetails = false
		m.processDetails = "" // Clear details
	}
	return m, nil
}

// updateSearchKey handles key events when the search input is focused in main list mode.
func (m model) updateSearchKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter, tea.KeyEsc:
		m.textInput.Blur()
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	m.filtered = m.filterProcesses(m.textInput.Value())
	return m, cmd
}

// updateMainListKey handles key events in the main process list mode.
// It returns handled=false for keys that should be passed to the generic handler.
func (m model) updateMainListKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit, true
	case tea.KeyCtrlR:
		return m, getProcesses, true
	case tea.KeyEsc:
		// ESC 退出模式：此处用于退出 ports-only 视图。
		if m.portsOnly {
			m.portsOnly = false
			m.filtered = m.filterProcesses(m.textInput.Value())
			return m, nil, true
		}
		// 未处于 ports-only 模式时视作未处理，交由外层处理。
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil, true
	case tea.KeyDown:
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return m, nil, true
	case tea.KeyEnter:
		if len(m.filtered) > 0 {
			p := m.filtered[m.cursor]
			return m, sendSignalWithStatus(int(p.Pid), syscall.SIGTERM, process.Killed), true
		}
		return m, nil, true
	}

	// rune-based main list commands
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'q':
			return m, tea.Quit, true
		case '?':
			m.helpOpen = true
			return m, nil, true
		case '/':
			m.textInput.Focus()
			return m, nil, true
		case 'P':
			// 进入“仅显示占用端口的进程”模式；退出由 ESC 统一处理。
			if !m.portsOnly {
				m.portsOnly = true
				m.filtered = m.filterProcesses(m.textInput.Value())
			}
			return m, nil, true
		case 'T':
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				m.dep.mode = true
				m.dep.rootPID = p.Pid
				if m.dep.expanded == nil {
					m.dep.expanded = make(map[int32]depNodeState)
				}
				m.dep.expanded[p.Pid] = depNodeState{expanded: true, page: 1}
				m.dep.cursor = 0
			}
			return m, nil, true
		case 'p':
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				return m, sendSignalWithStatus(int(p.Pid), syscall.SIGSTOP, process.Paused), true
			}
			return m, nil, true
		case 'r':
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				if p.Status == process.Paused {
					return m, sendSignalWithStatus(int(p.Pid), syscall.SIGCONT, process.Alive), true
				}
			}
			return m, nil, true
		case 'i':
			if len(m.filtered) > 0 {
				m.showDetails = true
				m.processDetails = ""
				p := m.filtered[m.cursor]
				return m, getProcessDetails(int(p.Pid)), true
			}
			return m, nil, true
		}
	}

	// Key not handled here; let outer handler decide.
	return m, nil, false
}
