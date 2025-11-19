package tui

import (
	"fmt"
	"strings"

	"github.com/w31r4/gokill/internal/process"

	"github.com/charmbracelet/lipgloss"
)

// --- UI 样式定义 ---
// 使用 `charmbracelet/lipgloss` 库来集中定义TUI的所有样式。
// 这种方式使得样式的管理、复用和主题化变得非常方便和清晰。
var (
	// docStyle 是整个应用窗口的基础样式，定义了外边距。
	docStyle = lipgloss.NewStyle().Margin(0, 1)
	// selectedStyle 定义了列表中当前光标选中行的样式，具有醒目的背景色和前景色。
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255"))
	// faintStyle 用于渲染次要信息（如帮助文本、状态标签），使其颜色变淡以降低视觉干扰。
	faintStyle = lipgloss.NewStyle().Faint(true)
	// killingStyle 定义了被标记为“已杀死”的进程的样式，使用删除线和红色来明确表示其状态。
	killingStyle = lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("9"))
	// pausedStyle 定义了被标记为“已暂停”的进程的样式，使用黄色进行提示。
	pausedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	// listeningStyle 定义了正在监听端口的进程的样式，同样使用黄色以引起注意。
	listeningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	// paneStyle 是所有面板（如进程列表、端口信息）的基础样式，定义了圆角边框和内边距。
	paneStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	// processPaneStyle 是左侧进程列表面板的专用样式，继承自 paneStyle 并设置了宽度和边框颜色。
	processPaneStyle = paneStyle.Copy().Width(60).BorderForeground(lipgloss.Color("62"))
	// portPaneStyle 是右侧端口信息面板的专用样式，同样继承自 paneStyle 并进行了定制。
	portPaneStyle = paneStyle.Copy().Width(16).BorderForeground(lipgloss.Color("220")).Align(lipgloss.Left)
	// detailTitleStyle 定义了详情视图的标题样式。
	detailTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	// detailPaneStyle 定义了详情视图的内容面板样式。
	detailPaneStyle = paneStyle.Copy().Width(80).BorderForeground(lipgloss.Color("63")).Padding(1, 2)
	// detailLabelStyle 定义了详情视图中标签（如 "PID:", "User:"）的样式，使其右对齐并加粗。
	detailLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true).Width(10).Align(lipgloss.Right)
	// detailValueStyle 定义了详情视图中值的样式。
	detailValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).MaxWidth(60)
	// detailHelpStyle 定义了详情视图底部的帮助文本样式。
	detailHelpStyle = faintStyle.Copy().MarginTop(1)
	// errorTitleStyle, errorPaneStyle, errorHelpStyle, errorMessageStyle 定义了错误覆盖层的各种样式。
	errorTitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	errorPaneStyle    = paneStyle.Copy().BorderForeground(lipgloss.Color("9")).Width(70).Padding(1, 2)
	errorHelpStyle    = faintStyle.Copy().MarginTop(1)
	errorMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	// confirm...Style 定义了确认对话框覆盖层的各种样式。
	confirmTitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true)
	confirmPaneStyle    = paneStyle.Copy().BorderForeground(lipgloss.Color("178")).Width(70).Padding(1, 2)
	confirmHelpStyle    = faintStyle.Copy().MarginTop(1)
	confirmMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	// help...Style 定义了帮助信息覆盖层的各种样式。
	helpTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	helpPaneStyle  = paneStyle.Copy().BorderForeground(lipgloss.Color("12")).Width(78).Padding(1, 2)
)

// 定义了不同列表视图的“视口”（Viewport）高度，即一次在屏幕上显示多少行。
const (
	viewHeight           = 7  // 主进程列表的视口高度。
	dependencyViewHeight = 18 // 依赖树视图的视口高度。
)

// View 是 Bubble Tea 架构中的核心渲染函数。
// 它根据当前的应用状态 `m` (model) 生成一个字符串，这个字符串就是即将在终端上显示的完整UI。
// Bubble Tea 的运行时会在每次 `Update` 函数返回后自动调用 `View` 来重绘界面。
//
// 这是一个“纯函数”，意味着它不应该有任何副作用（如修改模型或执行命令），
// 它的唯一职责就是忠实地将当前状态映射为可视化的字符串。
func (m model) View() string {
	// --- 视图调度逻辑 ---
	// 这里的 `if` 语句链定义了不同视图模式的渲染优先级。
	// 例如，一个错误信息应该覆盖所有其他视图，一个确认对话框应该覆盖主列表或依赖树。
	if m.err != nil {
		return m.renderErrorView()
	}
	if m.confirm != nil {
		return m.renderConfirmView()
	}
	if m.helpOpen {
		return m.renderHelpView()
	}
	if m.showDetails {
		return m.renderDetailsView()
	}
	if m.dep.mode {
		return m.renderDependencyView()
	}

	// --- 默认主列表视图的渲染 ---
	// 如果没有任何覆盖层或特殊模式被激活，则渲染主视图。
	if len(m.processes) == 0 {
		return "Loading processes..." // 在首次加载数据时显示一个加载提示。
	}

	// 组装主视图的各个部分：头部、内容区、底部。
	header := m.renderHeader()
	footer := m.renderFooter()

	// 如果过滤后没有任何结果，显示一个提示信息。
	if len(m.filtered) == 0 {
		noResults := "  No results..."
		return docStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, noResults, footer))
	}

	// 渲染主内容区，它由左侧的进程面板和右侧的端口面板水平拼接而成。
	processPane := m.renderProcessPane()
	portPane := m.renderPortPane()
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, processPane, portPane)

	// 将所有部分垂直拼接，并应用最外层的文档样式，最终返回完整的UI字符串。
	return docStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, mainContent, footer))
}

// --- 视图渲染函数 ---
// `View` 函数会调用这些辅助函数来构建UI的不同部分。

// renderDetailsView 负责渲染单个进程的详细信息视图。
// 这是一个全屏的覆盖视图。
func (m model) renderDetailsView() string {
	title := detailTitleStyle.Render("Process Details")
	var pane string
	// 当详情数据尚未加载完成时，显示一个加载提示。
	if m.processDetails == "" {
		pane = detailPaneStyle.Render(faintStyle.Render("Collecting details..."))
	} else {
		// 加载完成后，使用 `formatProcessDetails` 格式化并渲染详情内容。
		pane = detailPaneStyle.Render(formatProcessDetails(m.processDetails))
	}
	help := detailHelpStyle.Render(" esc/i/q: back to list")
	content := lipgloss.JoinVertical(lipgloss.Left, title, pane, help)
	return docStyle.Render(content)
}

// renderHeader 负责渲染应用的头部区域，通常包括标题、状态信息和搜索框。
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

// renderFooter 负责渲染应用的底部区域，通常用于显示上下文相关的帮助信息。
func (m model) renderFooter() string {
	var help strings.Builder
	// 根据搜索框是否激活，显示不同的提示。
	if m.textInput.Focused() {
		help.WriteString(faintStyle.Render(" enter/esc to exit search"))
	} else {
		// 在非搜索状态下，显示一个精简的核心操作指南。
		help.WriteString(faintStyle.Render("?: help • /: search • P: ports • T: tree • i: info • enter: kill • p: pause • r: resume • q: quit"))
	}
	return help.String()
}

// renderDependencyView 负责渲染全屏的依赖树视图。
func (m model) renderDependencyView() string {
	root := m.findProcess(m.dep.rootPID)
	if root == nil {
		title := detailTitleStyle.Render("Dependency Tree")
		hint := faintStyle.Render("(root process not found; esc to return)")
		return docStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, hint))
	}

	// Ancestor chain (optional)
	var anc []string
	if m.dep.showAncestors {
		anc = m.buildAncestorLines(root)
	}

	// 1. 构建并过滤扁平化的依赖树行列表。
	lines := applyDepFilters(m, buildDepLines(m))

	// 2. 计算视口，使其始终以当前光标为中心。
	start := m.dep.cursor - dependencyViewHeight/2
	if start < 0 {
		start = 0
	}
	end := start + dependencyViewHeight
	if end > len(lines) {
		end = len(lines)
		start = max(0, end-dependencyViewHeight)
	}

	// 3. 使用 strings.Builder 高效地构建视图字符串。
	var b strings.Builder
	title := detailTitleStyle.Render(fmt.Sprintf("Dependency Tree: %s (%d)", root.Executable, root.Pid))
	fmt.Fprintln(&b, title)

	// 4. 如果需要，渲染祖先链。
	if len(anc) > 0 {
		fmt.Fprintln(&b, faintStyle.Render("Ancestors"))
		for _, l := range anc {
			fmt.Fprintln(&b, faintStyle.Render(l))
		}
		fmt.Fprintln(&b, "")
	}

	// 5. 使用预计算的 childrenMap，避免每次渲染时重复构建。
	childrenMap := m.buildChildrenMap()
	for i := start; i < end; i++ {
		ln := lines[i]
		lineText := ln.text
		if it := m.findProcess(ln.pid); it != nil {
			// 检查是否存在未显示的子节点（因为深度限制或折叠），并据此添加一个 "+" 标记。
			hasKids := len(childrenMap[ln.pid]) > 0
			st := m.dep.expanded[ln.pid]
			allowDepth := dependencyTreeDepth - 1 + st.depthExtend
			hiddenDeps := hasKids && (!st.expanded || ln.depth >= allowDepth)

			// 应用进程状态样式。
			switch it.Status {
			case process.Killed:
				lineText = killingStyle.Render(lineText)
			case process.Paused:
				lineText = pausedStyle.Render(lineText)
			}
			if hiddenDeps {
				lineText += faintStyle.Render(" +")
			}
		}

		// 高亮显示当前光标所在的行。
		if i == m.dep.cursor {
			fmt.Fprintln(&b, selectedStyle.Render("❯ "+lineText))
		} else {
			fmt.Fprintln(&b, "  "+lineText)
		}
	}

	// 6. 构建并渲染底部的状态/帮助栏。
	var filterBadges []string
	if m.textInput.Value() != "" {
		filterBadges = append(filterBadges, fmt.Sprintf("filter: %q", m.textInput.Value()))
	}
	if m.dep.aliveOnly {
		filterBadges = append(filterBadges, "alive-only")
	}
	if m.dep.portsOnly {
		filterBadges = append(filterBadges, "listening-only")
	}
	filterStatus := ""
	if len(filterBadges) > 0 {
		filterStatus = faintStyle.Render(" [" + strings.Join(filterBadges, ", ") + "]")
	}

	help := faintStyle.Render("esc: back • /: filter • a: ancestors • s: alive • l: listen • u: up • enter/o: root • i: info" + filterStatus)
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
	if m.dep.mode {
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

// formatProcessDetails 是一个辅助函数，它接收 `GetProcessDetails` 返回的原始详情字符串，
// 并将其解析、格式化为一个美观的、带标签对齐的视图。
func formatProcessDetails(details string) string {
	lines := strings.Split(strings.TrimSpace(details), "\n")
	if len(lines) == 0 {
		return faintStyle.Render("(no details)")
	}

	var rows []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			rows = append(rows, "") // 保留空行以维持格式。
			continue
		}
		// 将 "Key: Value" 格式的行分割开。
		label, value := splitDetailLine(line)
		if label == "" {
			rows = append(rows, detailValueStyle.Render(value))
			continue
		}
		// 使用 lipgloss 样式分别渲染标签和值，然后水平拼接它们。
		labelCell := detailLabelStyle.Render(label + ":")
		valueCell := detailValueStyle.Render(value)
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, labelCell, " ", valueCell))
	}

	return strings.Join(rows, "\n")
}

// splitDetailLine 是一个健壮的辅助函数，用于将详情行分割为标签和值。
// 它能处理 ":\t" 和 ":" 两种分隔符。
func splitDetailLine(line string) (string, string) {
	if idx := strings.Index(line, ":\t"); idx != -1 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+2:])
	}
	if idx := strings.Index(line, ":"); idx != -1 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
	}
	return "", line
}

// friendlyErrorMessage 函数接收一个原始的 `error`，并尝试将其转换为一个对用户更友好的消息。
// 它通过匹配错误字符串中的常见模式（如权限问题、进程不存在等），来附加一些有用的提示信息。
func friendlyErrorMessage(err error) string {
	if err == nil {
		return "(n/a)"
	}

	raw := strings.TrimSpace(err.Error())
	lower := strings.ToLower(raw)

	switch {
	case strings.Contains(lower, "operation not permitted") || strings.Contains(lower, "permission denied"):
		return fmt.Sprintf("%s\n\nHint: Try running gokill with sudo or as an administrator.", raw)
	case strings.Contains(lower, "not found") || strings.Contains(lower, "no such process"):
		return fmt.Sprintf("%s\n\nHint: The process may have already exited. Try refreshing (ctrl+r).", raw)
	case strings.Contains(lower, "already finished"):
		return fmt.Sprintf("%s\n\nHint: The process exited just before the signal arrived. Refresh the list.", raw)
	default:
		return raw
	}
}
