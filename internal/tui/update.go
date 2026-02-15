package tui

import (
	"runtime"
	"strings"
	"syscall"

	"github.com/w31r4/gokill/internal/process"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmPrompt 结构体用于存储确认对话框所需的状态。
// 当用户执行一个危险操作（如杀死或暂停进程）时，我们会创建一个该结构体的实例，
// 并将其赋值给 `model.confirm`，从而触发确认视图的显示。
type confirmPrompt struct {
	pid           int32          // 目标进程的PID。
	name          string         // 目标进程的名称。
	op            string         // 操作类型的可读描述，如 "kill", "pause", "resume", "docker stop"。
	sig           syscall.Signal // 将要发送给进程的实际系统信号。
	status        process.Status // 操作成功后，进程应该更新到的新状态。
	containerName string         // Docker 容器名（非空时使用 docker stop）。
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
func getProcessDetails(pid int, requestID int64, opts process.DetailsOptions) tea.Cmd {
	return func() tea.Msg {
		// 在这个 Goroutine 中执行获取进程详情的耗时操作。
		details, err := process.GetProcessDetailsWithOptions(pid, opts)
		if err != nil {
			// 如果操作失败，返回一个 `errMsg` 消息，将错误传递给 Update 函数。
			return errMsg{err}
		}
		// 如果操作成功，返回一个 `processDetailsMsg` 消息，携带获取到的详情字符串。
		return processDetailsMsg{requestID: requestID, details: details}
	}
}

// Update 是 Bubble Tea 架构的核心，是应用的“状态机”。
// 它接收一个消息（`tea.Msg`），根据消息的类型来更新应用的状态模型（`model`），
// 并可以选择性地返回一个新的命令（`tea.Cmd`）来执行后续的异步操作。
// 整个应用的交互逻辑都集中在这里。
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case processesLoadedMsg:
		return m.updateProcessesLoaded(msg)
	case processDetailsMsg:
		return m.updateProcessDetails(msg)
	case errMsg:
		return m.updateErr(msg)
	case signalOKMsg:
		return m.updateSignalOK(msg)
	case tea.WindowSizeMsg:
		return m.updateWindowSize(msg), nil
	case tea.KeyMsg:
		newModel, keyCmd, handled := m.updateKeyMsg(msg)
		if handled {
			return newModel, keyCmd
		}
		m = newModel
	}

	return m.updateDefault(msg)
}

func (m model) updateProcessesLoaded(msg processesLoadedMsg) (tea.Model, tea.Cmd) {
	m.processes = msg.processes
	m.warnings = msg.warnings

	m.filtered = m.filterProcesses(m.textInput.Value())
	return m, func() tea.Msg {
		_ = process.Save(m.processes)
		return nil
	}
}

func (m model) updateProcessDetails(msg processDetailsMsg) (tea.Model, tea.Cmd) {
	if msg.requestID != m.detailsRequestID {
		return m, nil
	}

	m.processDetails = msg.details

	vpHFrame, _ := m.detailsViewport.Style.GetFrameSize()
	contentWidth := m.detailsViewport.Width - vpHFrame
	formattedDetails := formatProcessDetails(m.processDetails, contentWidth)

	lines := strings.Split(formattedDetails, "\n")
	contentHeight := len(lines)
	if contentHeight <= 0 {
		contentHeight = 1
	}
	maxHeight := m.detailsViewport.Height
	if maxHeight <= 0 {
		maxHeight = contentHeight
	}
	if contentHeight > maxHeight {
		m.detailsViewport.Height = maxHeight
	} else {
		m.detailsViewport.Height = contentHeight
	}

	m.detailsViewport.SetContent(formattedDetails)
	m.detailsViewport.GotoTop()
	return m, nil
}

func (m model) updateErr(msg errMsg) (tea.Model, tea.Cmd) {
	if !strings.Contains(msg.err.Error(), "process already finished") {
		m.err = msg.err
	}
	return m, nil
}

func (m model) updateSignalOK(msg signalOKMsg) (tea.Model, tea.Cmd) {
	for _, it := range m.processes {
		if int(it.Pid) == msg.pid {
			it.Status = msg.status
			break
		}
	}
	return m, nil
}

func (m model) updateWindowSize(msg tea.WindowSizeMsg) model {
	headerHeight := lipgloss.Height(detailTitleStyle.Render("Process Details"))
	footerHeight := lipgloss.Height(detailHelpStyle.Render(" esc: back to list • up/down/pgup/pgdn: scroll"))

	docHFrame, docVFrame := docStyle.GetFrameSize()
	paneHFrame, paneVFrame := detailPaneStyle.GetFrameSize()

	viewportWidth := msg.Width - docHFrame - paneHFrame
	viewportHeight := msg.Height - docVFrame - paneVFrame - headerHeight - footerHeight

	if viewportWidth < 0 {
		viewportWidth = 0
	}
	if viewportHeight < 0 {
		viewportHeight = 0
	}

	m.detailsViewport.Width = viewportWidth
	m.detailsViewport.Height = viewportHeight
	return m
}

func (m model) updateDefault(msg tea.Msg) (tea.Model, tea.Cmd) {
	var filterCmd tea.Cmd
	m.textInput, filterCmd = m.textInput.Update(msg)
	m.filtered = m.filterProcesses(m.textInput.Value())
	m.cursor = clampIndex(m.cursor, len(m.filtered))
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

// stopContainer 是用于停止 Docker 容器的命令工厂。
// 它调用 `docker stop` 而不是发送系统信号，因为 Docker 容器需要通过 Docker daemon 来正确停止。
func stopContainer(pid int, containerName string) tea.Cmd {
	return func() tea.Msg {
		if err := process.StopContainer(containerName); err != nil {
			return errMsg{err}
		}
		return signalOKMsg{pid: pid, status: process.Killed}
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
	// 模式的检查顺序需要与 View 的渲染优先级保持一致，避免“界面显示 A，但按键处理走 B”的状态错位。
	// Priority (high → low): error, confirm, help, details, dep-mode, search, main list.
	if m.err != nil {
		newModel, cmd := m.updateErrorKey(msg)
		return newModel, cmd, true
	}
	if m.confirm != nil {
		newModel, cmd := m.updateConfirmKey(msg)
		return newModel, cmd, true
	}
	if m.helpOpen {
		newModel, cmd := m.updateHelpKey(msg)
		return newModel, cmd, true
	}
	if m.showDetails {
		newModel, cmd := m.updateDetailsKey(msg)
		return newModel, cmd, true
	}
	if m.dep.mode {
		newModel, cmd, handled := m.updateDepModeKey(msg)
		if handled {
			return newModel, cmd, true
		}
		m = newModel
	}
	if m.textInput.Focused() {
		newModel, cmd := m.updateSearchKey(msg)
		return newModel, cmd, true
	}

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
		// 如果是 Docker 容器，使用 docker stop；否则发送系统信号。
		if op.containerName != "" {
			return m, stopContainer(int(op.pid), op.containerName)
		}
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
	if m.textInput.Focused() {
		return m.updateDepModeSearchKey(msg)
	}
	if newModel, cmd, handled := m.handleDepModeGlobalKey(msg); handled {
		return newModel, cmd, true
	}
	if newModel, cmd, handled := m.handleDepModeFilterKey(msg); handled {
		return newModel, cmd, true
	}
	if newModel, cmd, handled := m.handleDepModeRootKey(msg); handled {
		return newModel, cmd, true
	}
	if newModel, cmd, handled := m.handleDepModeActionKey(msg); handled {
		return newModel, cmd, true
	}
	if newModel, cmd, handled := m.handleDepModeNavKey(msg); handled {
		return newModel, cmd, true
	}

	return m, nil, false
}

func (m model) updateDepModeSearchKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	var cmd tea.Cmd
	switch msg.String() {
	case "enter", "esc":
		m.textInput.Blur()
	}
	m.textInput, cmd = m.textInput.Update(msg)
	m = m.clampDepCursor()
	return m, cmd, true
}

func (m model) handleDepModeGlobalKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		return m.exitDepMode(), nil, true
	case "ctrl+c", "q":
		return m, tea.Quit, true
	case "?":
		m.helpOpen = true
		return m, nil, true
	case "ctrl+r":
		return m, getProcesses, true
	}
	return m, nil, false
}

func (m model) handleDepModeFilterKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "/":
		m.textInput.Focus()
		return m, nil, true
	case "S":
		m.dep.aliveOnly = !m.dep.aliveOnly
		return m.clampDepCursor(), nil, true
	case "L":
		m.dep.portsOnly = !m.dep.portsOnly
		return m.clampDepCursor(), nil, true
	case "a":
		m.dep.showAncestors = !m.dep.showAncestors
		return m, nil, true
	}
	return m, nil, false
}

func (m model) handleDepModeRootKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "u":
		return m.depRootToParent(), nil, true
	case "enter", "o":
		if ln, ok := m.depLineAtCursor(); ok && ln.pid != 0 {
			m = m.setDepRoot(ln.pid)
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m model) handleDepModeActionKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "i":
		if ln, ok := m.depLineAtCursor(); ok && ln.pid != 0 {
			newModel, cmd := m.openProcessDetails(ln.pid)
			return newModel, cmd, true
		}
		return m, nil, true
	case "x":
		if ln, ok := m.depLineAtCursor(); ok {
			if it := m.findProcess(ln.pid); it != nil {
				if it.ContainerName != "" {
					m.confirm = &confirmPrompt{pid: ln.pid, name: it.ContainerName, op: "docker stop", sig: syscall.SIGTERM, status: process.Killed, containerName: it.ContainerName}
				} else {
					m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "kill", sig: syscall.SIGTERM, status: process.Killed}
				}
			}
		}
		return m, nil, true
	case "p":
		if ln, ok := m.depLineAtCursor(); ok {
			if it := m.findProcess(ln.pid); it != nil {
				m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "pause", sig: sigStop, status: process.Paused}
			}
		}
		return m, nil, true
	case "r":
		if ln, ok := m.depLineAtCursor(); ok {
			if it := m.findProcess(ln.pid); it != nil && it.Status == process.Paused {
				m.confirm = &confirmPrompt{pid: ln.pid, name: it.Executable, op: "resume", sig: sigCont, status: process.Alive}
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m model) handleDepModeNavKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "up", "k":
		m.dep.cursor--
		return m.clampDepCursor(), nil, true
	case "down", "j":
		m.dep.cursor++
		return m.clampDepCursor(), nil, true
	case "left", "h":
		return m.depCollapseAtCursor(), nil, true
	case "right", "l", " ":
		return m.depExpandAtCursor(msg.String()), nil, true
	}
	return m, nil, false
}

func (m model) depLineAtCursor() (depLine, bool) {
	lines := buildDepLines(m)
	if m.dep.cursor < 0 || m.dep.cursor >= len(lines) {
		return depLine{}, false
	}
	return lines[m.dep.cursor], true
}

func (m model) setDepRoot(pid int32) model {
	m.dep.rootPID = pid
	m.dep.expanded = map[int32]depNodeState{pid: {expanded: true, page: 1}}
	m.dep.cursor = 0
	return m
}

func (m model) depRootToParent() model {
	root := m.findProcess(m.dep.rootPID)
	if root == nil {
		return m
	}
	parent := m.findProcess(root.PPid)
	if parent == nil {
		return m
	}
	return m.setDepRoot(parent.Pid)
}

func (m model) exitDepMode() model {
	m.dep.mode = false
	m.dep.expanded = nil
	m.dep.cursor = 0
	return m
}

func (m model) depCollapseAtCursor() model {
	lines := buildDepLines(m)
	if m.dep.cursor < 0 || m.dep.cursor >= len(lines) {
		return m
	}

	ln := lines[m.dep.cursor]
	if ln.isMore {
		st := m.dep.expanded[ln.parent]
		st.expanded = false
		st.page = 1
		st.depthExtend = 0
		m.dep.expanded[ln.parent] = st
	} else if ln.pid != 0 {
		st := m.dep.expanded[ln.pid]
		st.expanded = false
		m.dep.expanded[ln.pid] = st
	}
	return m.clampDepCursor()
}

func (m model) depExpandAtCursor(key string) model {
	lines := buildDepLines(m)
	if m.dep.cursor < 0 || m.dep.cursor >= len(lines) {
		return m
	}

	ln := lines[m.dep.cursor]
	if ln.isMore {
		st := m.dep.expanded[ln.parent]
		if ln.isDeeper {
			st.depthExtend++
		} else {
			st.page++
		}
		st.expanded = true
		m.dep.expanded[ln.parent] = st
		return m.clampDepCursor()
	}
	if ln.pid == 0 {
		return m.clampDepCursor()
	}

	st := m.dep.expanded[ln.pid]
	st.expanded = !st.expanded
	if key == "right" || key == "l" {
		st.expanded = true
	}
	if st.page == 0 {
		st.page = 1
	}
	m.dep.expanded[ln.pid] = st
	return m.clampDepCursor()
}

func (m model) clampDepCursor() model {
	c := len(applyDepFilters(m, buildDepLines(m)))
	if c <= 0 {
		m.dep.cursor = 0
		return m
	}
	if m.dep.cursor < 0 {
		m.dep.cursor = 0
		return m
	}
	if m.dep.cursor >= c {
		m.dep.cursor = c - 1
	}
	return m
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
	case "esc":
		m.showDetails = false
		m.processDetails = "" // 清空详情内容，以便下次重新加载。
		m.detailsPID = 0
		m.detailsVerbose = false
		m.detailsShowEnv = false
		m.detailsRevealSecrets = false
	case "?":
		m.helpOpen = true
	case "ctrl+c":
		return m, tea.Quit
	case "v":
		m.detailsVerbose = !m.detailsVerbose
		return m.reloadProcessDetails()
	case "e":
		m.detailsShowEnv = !m.detailsShowEnv
		if !m.detailsShowEnv {
			m.detailsRevealSecrets = false
		}
		return m.reloadProcessDetails()
	case "s":
		// Secret reveal only applies when env is visible.
		if m.detailsShowEnv {
			m.detailsRevealSecrets = !m.detailsRevealSecrets
			return m.reloadProcessDetails()
		}
	}
	// 将按键转发给 viewport
	var cmd tea.Cmd
	m.detailsViewport, cmd = m.detailsViewport.Update(msg)
	return m, cmd
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
	if newModel, cmd, handled := m.handleMainListGlobalKey(msg); handled {
		return newModel, cmd, true
	}
	if newModel, cmd, handled := m.handleMainListViewKey(msg); handled {
		return newModel, cmd, true
	}
	if newModel, cmd, handled := m.handleMainListNavKey(msg); handled {
		return newModel, cmd, true
	}
	if newModel, cmd, handled := m.handleMainListActionKey(msg); handled {
		return newModel, cmd, true
	}

	return m, nil, false
}

func (m model) handleMainListGlobalKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit, true
	case "ctrl+r":
		return m, getProcesses, true
	}
	return m, nil, false
}

func (m model) handleMainListViewKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		if !m.portsOnly {
			return m, nil, false
		}
		m.portsOnly = false
		m.filtered = m.filterProcesses(m.textInput.Value())
		return m, nil, true
	case "?":
		m.helpOpen = true
		return m, nil, true
	case "/":
		m.textInput.Focus()
		return m, nil, true
	case "P":
		if !m.portsOnly {
			m.portsOnly = true
			m.filtered = m.filterProcesses(m.textInput.Value())
		}
		return m, nil, true
	case "T":
		if p, ok := m.selectedProcess(); ok {
			m = m.enterDepMode(p.Pid)
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m model) handleMainListNavKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
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
	}
	return m, nil, false
}

func (m model) handleMainListActionKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "enter":
		if p, ok := m.selectedProcess(); ok {
			if p.ContainerName != "" {
				// Docker containers go through confirm dialog since docker stop is a heavier operation.
				m.confirm = &confirmPrompt{pid: p.Pid, name: p.ContainerName, op: "docker stop", sig: syscall.SIGTERM, status: process.Killed, containerName: p.ContainerName}
				return m, nil, true
			}
			return m, sendSignalWithStatus(int(p.Pid), syscall.SIGTERM, process.Killed), true
		}
		return m, nil, false
	case "p":
		if p, ok := m.selectedProcess(); ok {
			return m, sendSignalWithStatus(int(p.Pid), sigStop, process.Paused), true
		}
		return m, nil, false
	case "r":
		if p, ok := m.selectedProcess(); ok && p.Status == process.Paused {
			return m, sendSignalWithStatus(int(p.Pid), sigCont, process.Alive), true
		}
		return m, nil, false
	case "i":
		if p, ok := m.selectedProcess(); ok {
			newModel, cmd := m.openProcessDetails(p.Pid)
			return newModel, cmd, true
		}
		return m, nil, false
	}
	return m, nil, false
}

func (m model) selectedProcess() (*process.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil, false
	}
	return m.filtered[m.cursor], true
}

func (m model) enterDepMode(pid int32) model {
	m.dep.mode = true
	m.dep.rootPID = pid
	if m.dep.expanded == nil {
		m.dep.expanded = make(map[int32]depNodeState)
	}
	m.dep.expanded[pid] = depNodeState{expanded: true, page: 1}
	m.dep.cursor = 0
	return m
}

func (m model) openProcessDetails(pid int32) (model, tea.Cmd) {
	m.showDetails = true
	m.processDetails = ""
	m.detailsPID = pid
	m.detailsVerbose = false
	m.detailsShowEnv = runtime.GOOS == "linux"
	m.detailsRevealSecrets = false
	m.detailsViewport.SetContent("Loading...")
	return m.reloadProcessDetails()
}

func (m model) reloadProcessDetails() (model, tea.Cmd) {
	if !m.showDetails || m.detailsPID <= 0 {
		return m, nil
	}

	m.detailsRequestID++
	m.detailsViewport.SetContent("Loading...")
	m.detailsViewport.GotoTop()

	opts := process.DetailsOptions{
		Verbose:          m.detailsVerbose,
		ShowEnv:          m.detailsShowEnv,
		RevealEnvSecrets: m.detailsRevealSecrets,
	}

	return m, getProcessDetails(int(m.detailsPID), m.detailsRequestID, opts)
}

// max 是一个简单的辅助函数，返回两个整数中的较大者。
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampIndex(idx, length int) int {
	if length <= 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= length {
		return length - 1
	}
	return idx
}
