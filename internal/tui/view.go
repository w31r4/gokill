package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/w31r4/gokill/internal/process"

	"github.com/charmbracelet/lipgloss"
)

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
	// detailTitleStyle 渲染详情模式标题。
	detailTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	// detailPaneStyle 为详情内容提供柔和的边框和内边距。
	detailPaneStyle = paneStyle.Copy().Width(80).BorderForeground(lipgloss.Color("63")).Padding(1, 2)
	// detailLabelStyle 对详情键名做对齐和强调。
	detailLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true).Width(10).Align(lipgloss.Right)
	// detailValueStyle 用于详情值。
	detailValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).MaxWidth(60)
	// detailHelpStyle 丰富详情模式提示。
	detailHelpStyle = faintStyle.Copy().MarginTop(1)
	// errorTitleStyle 渲染错误标题。
	errorTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	// errorPaneStyle 统一错误面板。
	errorPaneStyle = paneStyle.Copy().BorderForeground(lipgloss.Color("9")).Width(70).Padding(1, 2)
	// errorHelpStyle 提示错误视图退出方式。
	errorHelpStyle = faintStyle.Copy().MarginTop(1)
	// errorMessageStyle 用于高亮错误信息本体。
	errorMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	// confirm styles
	confirmTitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true)
	confirmPaneStyle    = paneStyle.Copy().BorderForeground(lipgloss.Color("178")).Width(70).Padding(1, 2)
	confirmHelpStyle    = faintStyle.Copy().MarginTop(1)
	confirmMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	// help styles
	helpTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	helpPaneStyle  = paneStyle.Copy().BorderForeground(lipgloss.Color("12")).Width(78).Padding(1, 2)
)

// 定义了进程列表视口（Viewport）的高度，即一次显示多少行。
const (
	viewHeight               = 7
	dependencyTreeDepth      = 3
	dependencyTreeChildLimit = 5
	dependencyViewHeight     = 18
	ancestorChainLimit       = 6
)

// View 函数根据当前的应用状态（model）生成一个字符串，用于在终端上显示。
// Bubble Tea 的运行时会不断调用这个函数来重绘界面。
// 这个函数应该是“纯”的，即不应有任何副作用，只负责根据 `m` 的数据渲染视图。
func (m model) View() string {
	// 如果模型中存在错误，则显示错误视图。
	if m.err != nil {
		return m.renderErrorView()
	}

	// 确认对话优先级次之（覆盖当前模式）。
	if m.confirm != nil {
		return m.renderConfirmView()
	}

	// Help overlay
	if m.helpOpen {
		return m.renderHelpView()
	}

	// 详情视图优先级高于其它模式（包括依赖模式）。
	if m.showDetails {
		return m.renderDetailsView()
	}

	// Dependency full-screen mode.
	if m.depMode {
		return m.renderDependencyView()
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
	title := detailTitleStyle.Render("Process Details")
	var pane string
	if m.processDetails == "" {
		pane = detailPaneStyle.Render(faintStyle.Render("Collecting details..."))
	} else {
		pane = detailPaneStyle.Render(formatProcessDetails(m.processDetails))
	}
	help := detailHelpStyle.Render(" esc: back to list")
	content := lipgloss.JoinVertical(lipgloss.Left, title, pane, help)
	return docStyle.Render(content)
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
		// 精简主界面帮助，更多按 ? 查看
		help.WriteString(faintStyle.Render(" ?: help • / search • P ports • ctrl+r refresh • i info • enter kill • p pause • r resume • q quit"))
	}
	return help.String()
}

// renderDependencyView 渲染全屏依赖树模式。
func (m model) renderDependencyView() string {
	root := m.findProcess(m.depRootPID)
	if root == nil {
		title := detailTitleStyle.Render("Dependency Tree")
		hint := faintStyle.Render("(root process not found; esc to return)")
		return docStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, hint))
	}

	// Ancestor chain (optional)
	var anc []string
	if m.depShowAncestors {
		anc = m.buildAncestorLines(root)
	}

	lines := applyDepFilters(m, buildDepLines(m))

	// 视口计算，围绕光标
	start := m.depCursor - dependencyViewHeight/2
	if start < 0 {
		start = 0
	}
	end := start + dependencyViewHeight
	if end > len(lines) {
		end = len(lines)
		start = end - dependencyViewHeight
		if start < 0 {
			start = 0
		}
	}

	var b strings.Builder
	title := detailTitleStyle.Render(fmt.Sprintf("Dependency Tree: %s (%d)", root.Executable, root.Pid))
	fmt.Fprintln(&b, title)
	if len(anc) > 0 {
		fmt.Fprintln(&b, faintStyle.Render("Ancestors"))
		for _, l := range anc {
			fmt.Fprintln(&b, faintStyle.Render(l))
		}
		fmt.Fprintln(&b, "")
	}

	childrenMap := m.buildChildrenMap()
	for i := start; i < end; i++ {
		ln := lines[i]
		lineText := ln.text
		if it := m.findProcess(ln.pid); it != nil {
			// Determine if this node has undisplayed dependencies
			hasKids := len(childrenMap[ln.pid]) > 0
			st := m.depExpanded[ln.pid]
			allowDepth := dependencyTreeDepth - 1 + st.depthExtend
			hiddenDeps := hasKids && (!st.expanded || ln.depth >= allowDepth)
			// apply status color
			switch it.Status {
			case process.Killed:
				lineText = killingStyle.Render(lineText)
			case process.Paused:
				lineText = pausedStyle.Render(lineText)
			}
			// append a subtle marker when there are hidden deps
			if hiddenDeps {
				lineText = lineText + faintStyle.Render(" +")
			}
			if i == m.depCursor {
				sel := selectedStyle
				if hiddenDeps {
					// add a faint hint inside selection too
					fmt.Fprintln(&b, sel.Render("❯ "+ln.text+faintStyle.Render(" +")))
				} else {
					fmt.Fprintln(&b, sel.Render("❯ "+ln.text))
				}
				continue
			}
		}
		if i == m.depCursor {
			fmt.Fprintln(&b, selectedStyle.Render("❯ "+ln.text))
		} else {
			fmt.Fprintln(&b, "  "+lineText)
		}
	}

	// Filter status line
	filterBadge := ""
	if m.textInput.Value() != "" {
		filterBadge = fmt.Sprintf(" [filter: %q]", m.textInput.Value())
	}
	if m.depAliveOnly {
		filterBadge += " [alive-only]"
	}
	if m.depPortsOnly {
		filterBadge += " [listening-only]"
	}

	help := faintStyle.Render(" ?: help • / filter • S alive • L listen • ←/→/space fold • enter/o root • u up • esc back • i details" + filterBadge)
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, help)
	return docStyle.Render(strings.TrimRight(b.String(), "\n"))
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
	fmt.Fprintln(&b, "Ports")

	// 如果没有进程或光标无效，则显示空状态，并提示 T 模式查看依赖树。
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		fmt.Fprintln(&b, faintStyle.Render("(n/a)"))
		fmt.Fprintln(&b, "")
		fmt.Fprintln(&b, faintStyle.Render("Press T to view dependency tree"))
		return portPaneStyle.Render(strings.TrimRight(b.String(), "\n"))
	}

	p := m.filtered[m.cursor]
	if len(p.Ports) == 0 {
		fmt.Fprintln(&b, faintStyle.Render("(none)"))
	} else {
		for _, port := range p.Ports {
			fmt.Fprintln(&b, fmt.Sprintf("%d", port))
		}
	}

	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, faintStyle.Render("Press T to view dependency tree"))

	return portPaneStyle.Render(strings.TrimRight(b.String(), "\n"))
}

// renderErrorView 专门渲染错误状态，提供可退出的视图。
func (m model) renderErrorView() string {
	title := errorTitleStyle.Render("Something went wrong")
	message := friendlyErrorMessage(m.err)
	body := errorPaneStyle.Render(errorMessageStyle.Render(message))
	help := errorHelpStyle.Render(" esc: dismiss • q: quit")
	return docStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, body, help))
}

// renderConfirmView 渲染操作确认对话。
func (m model) renderConfirmView() string {
	if m.confirm == nil {
		return ""
	}
	title := confirmTitleStyle.Render("Confirm Action")
	op := strings.Title(m.confirm.op)
	msg := fmt.Sprintf("Action: %s\nProcess: %s (%d)", op, m.confirm.name, m.confirm.pid)
	body := confirmPaneStyle.Render(confirmMessageStyle.Render(msg))
	help := confirmHelpStyle.Render(" y/enter: confirm • n/esc: cancel • q: quit")
	return docStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, body, help))
}

// renderHelpView 渲染帮助覆盖层，提供简洁的按键说明。
func (m model) renderHelpView() string {
	var b strings.Builder
	title := helpTitleStyle.Render("Help / Commands")
	fmt.Fprintln(&b, title)
	if m.depMode {
		fmt.Fprintln(&b, helpPaneStyle.Render(strings.Join([]string{
			"T-mode (dependency tree):",
			"  up/down (j/k): move cursor",
			"  left/right/space (h/l/space): fold/unfold; on ‘… (deeper)’ drill deeper; on ‘… (N more)’ page",
			"  enter/o: set current node as root; u: root up; a: toggle ancestors",
			"  /: filter • S: alive-only • L: listening-only",
			"  i: details • x: kill • p: pause • r: resume",
			"  esc: back • ctrl+r: refresh • ?: close help",
		}, "\n")))
	} else {
		fmt.Fprintln(&b, helpPaneStyle.Render(strings.Join([]string{
			"Main list:",
			"  up/down (j/k): move cursor",
			"  /: search • enter: kill • p: pause • r: resume • i: details",
			"  P: ports-only • ctrl+r: refresh • T: dependency tree",
			"  q/ctrl+c: quit • ?: close help",
		}, "\n")))
	}
	return docStyle.Render(strings.TrimRight(b.String(), "\n"))
}

// formatProcessDetails 将 GetProcessDetails 返回的原始字符串美化为带标签对齐的视图。
func formatProcessDetails(details string) string {
	lines := strings.Split(strings.TrimSpace(details), "\n")
	if len(lines) == 0 {
		return faintStyle.Render("(no details)")
	}

	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			rows = append(rows, "")
			continue
		}
		label, value := splitDetailLine(line)
		if label == "" {
			rows = append(rows, detailValueStyle.Render(value))
			continue
		}
		labelCell := detailLabelStyle.Render(label + ":")
		valueCell := detailValueStyle.Render(value)
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, labelCell, " ", valueCell))
	}

	return strings.Join(rows, "\n")
}

func splitDetailLine(line string) (string, string) {
	if idx := strings.Index(line, ":\t"); idx != -1 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+2:])
	}
	if idx := strings.Index(line, ":"); idx != -1 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
	}
	return "", line
}

// friendlyErrorMessage 为常见错误提供更直观的提示，同时保留原始信息。
func friendlyErrorMessage(err error) string {
	if err == nil {
		return "(n/a)"
	}

	raw := strings.TrimSpace(err.Error())
	lower := strings.ToLower(raw)

	switch {
	case strings.Contains(lower, "operation not permitted") || strings.Contains(lower, "permission denied"):
		return fmt.Sprintf("%s\n\nHint: try running gokill with sudo or target processes owned by your user.", raw)
	case strings.Contains(lower, "not found") || strings.Contains(lower, "no such process"):
		return fmt.Sprintf("%s\n\nHint: the process may have already exited. Refresh (ctrl+r) and try again.", raw)
	case strings.Contains(lower, "already finished"):
		return fmt.Sprintf("%s\n\nHint: that PID exited just before the signal arrived. Refresh the list.", raw)
	default:
		return raw
	}
}

// renderDependencyTree 根据当前选中的进程构建一个类似 tree 的依赖结构。
func (m model) renderDependencyTree(root *process.Item) string {
	if root == nil || len(m.processes) == 0 {
		return ""
	}

	children := make(map[int32][]*process.Item)
	for _, it := range m.processes {
		children[it.PPid] = append(children[it.PPid], it)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d)\n", root.Executable, root.Pid)
	renderDependencyChildren(&b, children, root.Pid, "", 0)

	return strings.TrimRight(b.String(), "\n")
}

func renderDependencyChildren(b *strings.Builder, children map[int32][]*process.Item, pid int32, prefix string, depth int) {
	if depth >= dependencyTreeDepth-1 {
		return
	}

	kids := children[pid]
	if len(kids) == 0 {
		return
	}

	sort.Slice(kids, func(i, j int) bool {
		if kids[i].Executable == kids[j].Executable {
			return kids[i].Pid < kids[j].Pid
		}
		return kids[i].Executable < kids[j].Executable
	})

	truncated := 0
	if len(kids) > dependencyTreeChildLimit {
		truncated = len(kids) - dependencyTreeChildLimit
		kids = kids[:dependencyTreeChildLimit]
	}

	for i, child := range kids {
		last := i == len(kids)-1 && truncated == 0
		fmt.Fprintf(b, "%s%s %s (%d)\n", prefix, branchSymbol(last), child.Executable, child.Pid)
		nextPrefix := prefix
		if last {
			nextPrefix += "   "
		} else {
			nextPrefix += "│  "
		}
		renderDependencyChildren(b, children, child.Pid, nextPrefix, depth+1)
	}

	if truncated > 0 {
		fmt.Fprintf(b, "%s└─ ... (%d more)\n", prefix, truncated)
	}
}

