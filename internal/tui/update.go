package tui

import (
	"strings"
	"syscall"

	"github.com/w31r4/gokill/internal/process"

	tea "github.com/charmbracelet/bubbletea"
)

// confirmPrompt 结构体用于存储确认对话框所需的状态。
// 当用户执行一个危险操作（如杀死或暂停进程）时，我们会创建一个该结构体的实例，
// 并将其赋值给 `model.confirm`，从而触发确认视图的显示。
type confirmPrompt struct {
	pid    int32          // 目标进程的PID。
	name   string         // 目标进程的名称。
	op     string         // 操作类型的可读描述，如 "kill", "pause", "resume"。
	sig    syscall.Signal // 将要发送给进程的实际系统信号。
	status process.Status // 操作成功后，进程应该更新到的新状态。
}

// Init 是 Bubble Tea 应用生命周期的一部分，在程序首次运行时被调用。
// 它负责返回一个或多个初始命令（`tea.Cmd`）来启动应用的异步任务。
func (m model) Init() tea.Cmd {
	// `tea.Batch` 是一个辅助函数，用于将多个命令合并成一个，以便它们可以并发执行。
	// 这里我们同时执行两个初始任务：
	// 1. `m.textInput.Focus()`: 使搜索框立即获得焦点，方便用户直接输入。
	// 2. `getProcesses`: 触发一个异步命令来从系统中获取最新的进程列表。
	return tea.Batch(m.textInput.Focus(), getProcesses)
}

// getProcesses 是一个命令（`tea.Cmd`），它封装了获取系统进程列表的耗时操作。
// 在 Bubble Tea 中，任何可能阻塞UI的操作都应该包装在一个命令中。
// 命令本质上是一个函数，其返回值必须是 `tea.Msg`。Bubble Tea 运行时会
// 在一个单独的 Goroutine 中执行这个函数，并将返回的消息发送给 `Update` 方法。
func getProcesses() tea.Msg {
	procs, warnings, err := process.GetProcesses()
	if err != nil {
		return errMsg{err}
	}
	return processesLoadedMsg{processes: procs, warnings: warnings}
}

// getProcessDetails 是一个命令工厂函数。它接收一个进程PID作为参数，
// 并返回一个具体的 `tea.Cmd`（一个闭包函数）。这种模式使得创建带参数的命令变得非常方便。
func getProcessDetails(pid int) tea.Cmd {
	return func() tea.Msg {
		// 在这个 Goroutine 中执行获取进程详情的耗时操作。
		details, err := process.GetProcessDetails(pid)
		if err != nil {
			// 如果操作失败，返回一个 `errMsg` 消息，将错误传递给 Update 函数。
			return errMsg{err}
		}
		// 如果操作成功，返回一个 `processDetailsMsg` 消息，携带获取到的详情字符串。
		return processDetailsMsg(details)
	}
}

// Update 是 Bubble Tea 架构的核心，是应用的“状态机”。
// 它接收一个消息（`tea.Msg`），根据消息的类型来更新应用的状态模型（`model`），
// 并可以选择性地返回一个新的命令（`tea.Cmd`）来执行后续的异步操作。
// 整个应用的交互逻辑都集中在这里。
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 使用一个 `switch` 语句来判断传入消息的具体类型。
	switch msg := msg.(type) {
	// 消息1: 进程列表已成功加载。
	case processesLoadedMsg:
		m.processes = msg.processes // 使用新列表更新模型中的完整进程列表。
		m.warnings = msg.warnings   // 存储获取过程中产生的任何警告。



		m.filtered = m.filterProcesses(m.textInput.Value()) // 根据当前的搜索词重新过滤列表。
		// 返回一个命令，在后台异步地将新的进程列表保存到缓存文件。
		// 这是一个“即发即忘”的命令，我们不关心它的结果。
		return m, func() tea.Msg {
			_ = process.Save(m.processes)
			return nil
		}

	// 消息2: 单个进程的详细信息已加载。
	case processDetailsMsg:
		m.processDetails = string(msg) // 更新模型中的详情字符串。
		return m, nil                  // 无需执行新的命令。

	// 消息3: 发生了一个错误。
	case errMsg:
		// 在并发环境中，我们可能会尝试操作一个刚刚退出的进程，
		// 这会产生 "process already finished" 错误。这是一种正常情况，
		// 我们选择忽略它，以避免不必要的用户干扰。
		if !strings.Contains(msg.err.Error(), "process already finished") {
			m.err = msg.err // 对于所有其他错误，将其存储在模型中以便在UI上显示。
		}
		return m, nil

	// 消息4: 发送信号的操作已成功完成。
	case signalOKMsg:
		// 遍历完整的进程列表，找到匹配PID的进程并更新其状态。
		for _, it := range m.processes {
			if int(it.Pid) == msg.pid {
				it.Status = msg.status
				break
			}
		}
		// `m.filtered` 列表中的项是指向 `m.processes` 中元素的指针，
		// 因此这里的修改会自动反映在过滤后的列表中。我们只需返回模型，
		// Bubble Tea 的运行时会自动调用 `View` 函数来重绘界面。
		return m, nil

	// 消息5: 用户按键输入。
	case tea.KeyMsg:
		// 将按键消息分发给一个专门的、基于当前UI模式的处理器。
		newModel, keyCmd, handled := m.updateKeyMsg(msg)
		if handled {
			// 如果按键已被子处理器完全处理，则直接返回其结果。
			return newModel, keyCmd
		}
		// 如果未被处理（例如，在搜索框中输入字符），则进入下面的默认处理逻辑。
	}

	// --- 默认处理逻辑 ---
	// 这部分主要处理当按键消息没有被 `updateKeyMsg` 的特定模式捕获时的情况，
	// 通常意味着用户正在与搜索框交互。

	var filterCmd tea.Cmd
	// 将消息传递给 `textinput` 组件的 `Update` 方法，让它处理输入。
	m.textInput, filterCmd = m.textInput.Update(msg)

	// 每当搜索框内容变化时，都重新执行过滤。
	m.filtered = m.filterProcesses(m.textInput.Value())

	// 过滤后，列表长度可能变化，需要确保光标位置仍然有效。
	// 这被称为“钳制”（Clamping）光标。
	if m.cursor >= len(m.filtered) {
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		} else {
			m.cursor = 0
		}
	}

	return m, filterCmd
}

// sendSignal 是一个简单的命令工厂，用于创建一个发送信号的命令。
// 这个命令是“即发即忘”的，它不关心操作是否成功，也不会在成功后返回任何消息来更新UI。
// 它只在失败时返回一个 `errMsg`。
func sendSignal(pid int, sig syscall.Signal) tea.Cmd {
	return func() tea.Msg {
		if err := process.SendSignal(pid, sig); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

// sendSignalWithStatus 是一个更完善的命令工厂，它实现了“请求-响应”模式。
// 它不仅发送信号，而且在成功后会返回一条 `signalOKMsg` 消息。
// `Update` 函数接收到这条消息后，才会安全地更新UI中进程的状态。
// 这种方式确保了UI状态的变更总是基于已确认的成功操作。
func sendSignalWithStatus(pid int, sig syscall.Signal, status process.Status) tea.Cmd {
	return func() tea.Msg {
		if err := process.SendSignal(pid, sig); err != nil {
			return errMsg{err}
		}
		return signalOKMsg{pid: pid, status: status}
	}
}

// updateKeyMsg 是一个关键的调度函数，它根据当前的UI模式（如帮助、确认、依赖树等）
// 将接收到的 `tea.KeyMsg` 分发给相应的子处理函数。
// 这种分层处理使得每个模式的按键逻辑可以被独立管理，大大降低了 `Update` 函数的复杂性。
//
// 返回值:
//   - `model`: 更新后的模型。
//   - `tea.Cmd`: 可能需要执行的新命令。
//   - `bool`: `true` 表示按键已被当前模式完全处理；`false` 表示需要交由后续的默认逻辑处理。
func (m model) updateKeyMsg(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	// 模式的检查顺序定义了它们的优先级。例如，帮助和确认对话框应该覆盖所有其他视图。
	// 1. 帮助覆盖层
	if m.helpOpen {
		newModel, cmd := m.updateHelpKey(msg)
		return newModel, cmd, true
	}

	// 2. 确认对话框覆盖层
	if m.confirm != nil {
		newModel, cmd := m.updateConfirmKey(msg)
		return newModel, cmd, true
	}

	// 3. 依赖树模式 (T模式)
	if m.dep.mode {
		newModel, cmd, handled := m.updateDepModeKey(msg)
		if handled {
			return newModel, cmd, true
		}
		// 如果 T 模式没有完全处理该按键，它可能仍然修改了模型（例如，切换了内部过滤器），
		// 所以我们需要使用它返回的新模型继续。
		m = newModel
	}

	// 4. 错误信息覆盖层
	if m.err != nil {
		newModel, cmd := m.updateErrorKey(msg)
		return newModel, cmd, true
	}

	// 5. 进程详情视图
	if m.showDetails {
		newModel, cmd := m.updateDetailsKey(msg)
		return newModel, cmd, true
	}

	// 6. 主列表视图下的搜索框激活状态
	if m.textInput.Focused() {
		newModel, cmd := m.updateSearchKey(msg)
		return newModel, cmd, true
	}

	// 7. 如果以上所有模式都未激活，则使用主列表的默认按键处理逻辑。
	return m.updateMainListKey(msg)
}

// updateHelpKey 专门处理当帮助覆盖层 (`m.helpOpen == true`) 显示时的按键事件。
func (m model) updateHelpKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch key := msg.String(); key {
	case "?", "esc":
		m.helpOpen = false // 关闭帮助视图
		return m, nil
	case "ctrl+c", "q":
		return m, tea.Quit // 退出程序
	}
	return m, nil
}

// updateConfirmKey 专门处理当确认对话框 (`m.confirm != nil`) 显示时的按键事件。
func (m model) updateConfirmKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch key := msg.String(); key {
	case "y", "enter":
		op := *m.confirm // 复制确认操作的上下文
		m.confirm = nil  // 清除确认状态，关闭对话框
		// 返回一个命令来实际执行信号发送
		return m, sendSignalWithStatus(int(op.pid), op.sig, op.status)
	case "n", "esc":
		m.confirm = nil // 取消操作，关闭对话框
		return m, nil
	case "ctrl+c", "q":
		return m, tea.Quit // 退出程序
	}
	return m, nil
}

// updateDepModeKey 专门处理当应用处于“依赖树模式” (`m.dep.mode == true`) 时的按键事件。
// 这是最复杂的一个按键处理器，因为它包含了树的导航、过滤、展开/折叠以及对节点执行操作的逻辑。
func (m model) updateDepModeKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	var cmd tea.Cmd

	// 优先处理搜索框激活的情况。
	if m.textInput.Focused() {
		switch msg.String() {
		case "enter", "esc":
			m.textInput.Blur() // 退出搜索框焦点。
		}
		m.textInput, cmd = m.textInput.Update(msg)
		// 每次输入变化后，重新计算扁平化和过滤后的行，并钳制光标。
		if c := len(applyDepFilters(m, buildDepLines(m))); m.dep.cursor >= c {
			m.dep.cursor = max(0, c-1)
		}
		return m, cmd, true
	}

	// 处理非搜索状态下的按键。
	switch key := msg.String(); key {
	// 视图切换与退出
	case "esc":
		m.dep.mode = false   // 退出依赖树模式。
		m.dep.expanded = nil // 清空展开状态。
		m.dep.cursor = 0     // 重置光标。
		return m, nil, true
	case "ctrl+c", "q":
		return m, tea.Quit, true
	case "?":
		m.helpOpen = true
		return m, nil, true
	case "ctrl+r":
		return m, getProcesses, true // 刷新整个进程列表。

	// 过滤与显示选项
	case "/":
		m.textInput.Focus() // 激活搜索框。
		return m, nil, true
	case "S":
		m.dep.aliveOnly = !m.dep.aliveOnly // 切换“仅显示存活”过滤器。
		if c := len(applyDepFilters(m, buildDepLines(m))); m.dep.cursor >= c {
			m.dep.cursor = max(0, c-1)
		}
		return m, nil, true
	case "L":
		m.dep.portsOnly = !m.dep.portsOnly // 切换“仅显示监听端口”过滤器。
		if c := len(applyDepFilters(m, buildDepLines(m))); m.dep.cursor >= c {
			m.dep.cursor = max(0, c-1)
		}
		return m, nil, true
	case "a":
		m.dep.showAncestors = !m.dep.showAncestors // 切换是否显示祖先链。
		return m, nil, true

	// 树结构导航
	case "u": // 将父进程设为新的根节点。
		if root := m.findProcess(m.dep.rootPID); root != nil {
			if parent := m.findProcess(root.PPid); parent != nil {
				m.dep.rootPID = parent.Pid
				m.dep.expanded = map[int32]depNodeState{parent.Pid: {expanded: true, page: 1}}
				m.dep.cursor = 0
			}
		}
		return m, nil, true
	case "enter", "o": // 将当前选中的节点设为新的根节点。
		lines := buildDepLines(m)
		if len(lines) > 0 && m.dep.cursor < len(lines) {
			ln := lines[m.dep.cursor]
			if ln.pid != 0 {
				m.dep.rootPID = ln.pid
				m.dep.expanded = map[int32]depNodeState{ln.pid: {expanded: true, page: 1}}
				m.dep.cursor = 0
			}
		}
		return m, nil, true

	// 对选中节点执行操作
	case "i": // 显示详情
		lines := buildDepLines(m)
		if len(lines) > 0 && m.dep.cursor < len(lines) {
			ln := lines[m.dep.cursor]
			if ln.pid != 0 {
				m.showDetails = true
				m.processDetails = ""
				return m, getProcessDetails(int(ln.pid)), true
			}
		}
		return m, nil, true
	case "x": // 杀死进程
		lines := buildDepLines(m)
		if len(lines) > 0 && m.dep.cursor < len(lines) {
			ln := lines[m.dep.cursor]
			if it := m.findProcess(ln.pid); it != nil {
				m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "kill", sig: syscall.SIGTERM, status: process.Killed}
			}
		}
		return m, nil, true
	case "p": // 暂停进程
		lines := buildDepLines(m)
		if len(lines) > 0 && m.dep.cursor < len(lines) {
			ln := lines[m.dep.cursor]
			if it := m.findProcess(ln.pid); it != nil {
				m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "pause", sig: syscall.SIGSTOP, status: process.Paused}
			}
		}
		return m, nil, true
	case "r": // 恢复进程
		lines := buildDepLines(m)
		if len(lines) > 0 && m.dep.cursor < len(lines) {
			ln := lines[m.dep.cursor]
			if it := m.findProcess(ln.pid); it != nil && it.Status == process.Paused {
				m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "resume", sig: syscall.SIGCONT, status: process.Alive}
			}
		}
		return m, nil, true

	// 光标与折叠/展开
	case "up", "k":
		if m.dep.cursor > 0 {
			m.dep.cursor--
		}
		return m, nil, true
	case "down", "j":
		if m.dep.cursor < len(applyDepFilters(m, buildDepLines(m)))-1 {
			m.dep.cursor++
		}
		return m, nil, true
	case "left", "h": // 折叠节点
		lines := buildDepLines(m)
		if len(lines) > 0 && m.dep.cursor < len(lines) {
			ln := lines[m.dep.cursor]
			if ln.isMore { // 如果在 "more" 或 "deeper" 上，则折叠其父节点。
				st := m.dep.expanded[ln.parent]
				st.expanded = false
				st.page = 1
				st.depthExtend = 0
				m.dep.expanded[ln.parent] = st
			} else if ln.pid != 0 { // 否则折叠当前节点。
				st := m.dep.expanded[ln.pid]
				st.expanded = false
				m.dep.expanded[ln.pid] = st
			}
			// 钳制光标
			if c := len(applyDepFilters(m, buildDepLines(m))); m.dep.cursor >= c {
				m.dep.cursor = max(0, c-1)
			}
		}
		return m, nil, true
	case "right", "l", " ": // 展开节点、分页或钻取
		lines := buildDepLines(m)
		if len(lines) > 0 && m.dep.cursor < len(lines) {
			ln := lines[m.dep.cursor]
			if ln.isMore { // 如果在提示行上
				st := m.dep.expanded[ln.parent]
				if ln.isDeeper {
					st.depthExtend++ // 增加钻取深度
				} else {
					st.page++ // 加载下一页
				}
				st.expanded = true
				m.dep.expanded[ln.parent] = st
			} else if ln.pid != 0 { // 在普通节点上，展开它。
				st := m.dep.expanded[ln.pid]
				st.expanded = !st.expanded // 空格键可以切换展开/折叠
				if key == "right" || key == "l" {
					st.expanded = true // 方向键总是展开
				}
				if st.page == 0 {
					st.page = 1
				}
				m.dep.expanded[ln.pid] = st
			}
			// 钳制光标
			if c := len(applyDepFilters(m, buildDepLines(m))); m.dep.cursor >= c {
				m.dep.cursor = max(0, c-1)
			}
		}
		return m, nil, true
	}

	// 如果按键在 T 模式下未被处理，返回 false，让上层 Update 函数的默认逻辑处理。
	return m, nil, false
}

// updateErrorKey 专门处理当错误覆盖层 (`m.err != nil`) 显示时的按键事件。
func (m model) updateErrorKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.err = nil // 清除错误，关闭覆盖层。
		return m, nil
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	return m, nil
}

// updateDetailsKey 专门处理当进程详情视图 (`m.showDetails == true`) 显示时的按键事件。
func (m model) updateDetailsKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "i":
		m.showDetails = false
		m.processDetails = "" // 清空详情内容，以便下次重新加载。
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// updateSearchKey 专门处理当主列表的搜索框被激活 (`m.textInput.Focused()`) 时的按键事件。
func (m model) updateSearchKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.textInput.Blur() // 退出搜索框焦点。
	}
	// 将按键消息传递给 textinput 组件自身来处理文本输入。
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	// 每次输入变化都重新过滤列表。
	m.filtered = m.filterProcesses(m.textInput.Value())
	return m, cmd
}

// updateMainListKey 处理在主进程列表视图中的默认按键事件。
// 这是当所有其他特定模式（如帮助、确认、依赖树等）都未激活时的最终处理器。
func (m model) updateMainListKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch key := msg.String(); key {
	// 退出与刷新
	case "ctrl+c", "q":
		return m, tea.Quit, true
	case "ctrl+r":
		return m, getProcesses, true

	// 视图与模式切换
	case "esc":
		// 在主列表中，ESC 的主要作用是退出 "ports-only" 模式。
		if m.portsOnly {
			m.portsOnly = false
			m.filtered = m.filterProcesses(m.textInput.Value())
			return m, nil, true
		}
		// 如果不在该模式，则此按键未被处理。
	case "?":
		m.helpOpen = true
		return m, nil, true
	case "/":
		m.textInput.Focus()
		return m, nil, true
	case "P": // 注意是大写P
		// 进入“仅显示占用端口的进程”模式。
		if !m.portsOnly {
			m.portsOnly = true
			m.filtered = m.filterProcesses(m.textInput.Value())
		}
		return m, nil, true
	case "T": // 注意是大写T
		// 基于当前选中的进程，进入依赖树模式。
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
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

	// 列表导航
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil, true
	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return m, nil, true

	// 对选中进程执行操作
	case "enter": // 默认操作：杀死进程
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			p := m.filtered[m.cursor]
			// 注意：在主列表视图中，此操作默认直接发送信号，不弹出确认框，以实现快速操作。
			// 这与依赖树视图中的行为不同，后者总是要求确认。
			return m, sendSignalWithStatus(int(p.Pid), syscall.SIGTERM, process.Killed), true
		}
	case "p": // 暂停进程
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			p := m.filtered[m.cursor]
			// 直接发送 SIGSTOP 信号。
			return m, sendSignalWithStatus(int(p.Pid), syscall.SIGSTOP, process.Paused), true
		}
	case "r": // 恢复进程
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			p := m.filtered[m.cursor]
			if p.Status == process.Paused {
				return m, sendSignalWithStatus(int(p.Pid), syscall.SIGCONT, process.Alive), true
			}
		}
	case "i": // 显示详情
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			m.showDetails = true
			m.processDetails = ""
			p := m.filtered[m.cursor]
			return m, getProcessDetails(int(p.Pid)), true
		}
	}

	// 如果按键到这里仍未被处理，返回 false，让上层 Update 的默认逻辑（即文本输入）处理。
	return m, nil, false
}

// max 是一个简单的辅助函数，返回两个整数中的较大者。
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
