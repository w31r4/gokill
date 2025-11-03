package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/w31r4/gokill/internal/process"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// A message containing the list of processes.
type processesMsg []*process.Item

// A message containing the detailed info of a process.
type processDetailsMsg string

// A message containing an error.
type errMsg struct{ err error }

// model a struct to hold our application's state
type model struct {
	processes      []*process.Item
	filtered       []*process.Item
	cursor         int
	textInput      textinput.Model
	err            error
	showDetails    bool
	processDetails string
}

// InitialModel returns the initial model for the program
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

func (m model) Init() tea.Cmd {
	return tea.Batch(m.textInput.Focus(), getProcesses)
}

// getProcesses is a tea.Cmd that gets the list of processes.
func getProcesses() tea.Msg {
	procs, err := process.GetProcesses()
	if err != nil {
		return errMsg{err}
	}
	return processesMsg(procs)
}

func getProcessDetails(pid int) tea.Cmd {
	return func() tea.Msg {
		details, err := process.GetProcessDetails(pid)
		if err != nil {
			return errMsg{err}
		}
		return processDetailsMsg(details)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case processesMsg:
		m.processes = msg
		m.filtered = m.filterProcesses(m.textInput.Value())
		return m, func() tea.Msg {
			_ = process.Save(m.processes)
			return nil
		}

	case processDetailsMsg:
		m.processDetails = string(msg)
		return m, nil

	case errMsg:
		// We're just going to display the error for now, unless it's a
		// "process already finished" error, in which case we'll just
		// ignore it.
		if !strings.Contains(msg.err.Error(), "process already finished") {
			m.err = msg.err
		}
		return m, nil

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
				p.Status = process.Killed
				return m, sendSignal(int(p.Pid), syscall.SIGTERM)
			}
		case "p":
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				p.Status = process.Paused
				return m, sendSignal(int(p.Pid), syscall.SIGSTOP)
			}
		case "r":
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				if p.Status == process.Paused {
					p.Status = process.Alive
					return m, sendSignal(int(p.Pid), syscall.SIGCONT)
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

// fuzzyProcessSource wraps the process list to implement the fuzzy.Source interface.
type fuzzyProcessSource struct {
	processes []*process.Item
}

// String returns the string to be matched for the item at index i.
// We combine executable, PID, and ports to allow searching on all of them.
func (s fuzzyProcessSource) String(i int) string {
	p := s.processes[i]
	if ports := portsForSearch(p.Ports); ports != "" {
		return fmt.Sprintf("%s %d %s", p.Executable, p.Pid, ports)
	}
	return fmt.Sprintf("%s %d", p.Executable, p.Pid)
}

// Len returns the number of items in the collection.
func (s fuzzyProcessSource) Len() int {
	return len(s.processes)
}

func (m *model) filterProcesses(filter string) []*process.Item {
	var filtered []*process.Item
	// If the filter is empty, return all non-killed processes.
	if filter == "" {
		for _, p := range m.processes {
			if p.Status != process.Killed {
				filtered = append(filtered, p)
			}
		}
		return filtered
	}

	// Use the fuzzy finder to get ranked matches.
	source := fuzzyProcessSource{processes: m.processes}
	matches := fuzzy.FindFrom(filter, source)

	// Build the filtered list from the matches.
	for _, match := range matches {
		p := m.processes[match.Index]
		if p.Status != process.Killed {
			filtered = append(filtered, p)
		}
	}

	return filtered
}

func sendSignal(pid int, sig syscall.Signal) tea.Cmd {
	return func() tea.Msg {
		if err := process.SendSignal(pid, sig); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

var (
	docStyle         = lipgloss.NewStyle().Margin(1, 2)
	selectedStyle    = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255"))
	faintStyle       = lipgloss.NewStyle().Faint(true)
	killingStyle     = lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("9"))
	pausedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	listeningStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	paneStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	processPaneStyle = paneStyle.Copy().Width(60).BorderForeground(lipgloss.Color("62"))
	portPaneStyle    = paneStyle.Copy().Width(20).BorderForeground(lipgloss.Color("220"))
)

const (
	viewHeight = 7
)

func (m model) View() string {
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

func (m model) renderDetailsView() string {
	var b strings.Builder
	fmt.Fprintln(&b, "Process Details")
	if m.processDetails == "" {
		fmt.Fprintln(&b, "\n  Loading...")
	} else {
		fmt.Fprintln(&b, "")
		fmt.Fprint(&b, m.processDetails)
	}
	help := faintStyle.Render("\n\n q/esc/i: back to list")
	fmt.Fprint(&b, help)
	return docStyle.Render(b.String())
}

func (m model) renderHeader() string {
	count := fmt.Sprintf("(%d/%d)", len(m.filtered), len(m.processes))
	return fmt.Sprintf("Search processes/ports %s: %s", faintStyle.Render(count), m.textInput.View())
}

func (m model) renderFooter() string {
	var help strings.Builder
	if m.textInput.Focused() {
		help.WriteString(faintStyle.Render(" enter/esc to exit search"))
	} else {
		help.WriteString(faintStyle.Render(" /: search • i: info • ctrl+r: refresh • r: resume • p: pause • enter: kill • q: quit"))
	}
	return help.String()
}

func (m model) renderProcessPane() string {
	var b strings.Builder

	// Viewport calculation
	start := m.cursor - viewHeight/2
	if start < 0 {
		start = 0
	}
	end := start + viewHeight
	if end > len(m.filtered) {
		end = len(m.filtered)
		start = end - viewHeight
		if start < 0 {
			start = 0
		}
	}

	// Render viewport
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

	return processPaneStyle.Render(b.String())
}

func (m model) renderPortPane() string {
	var b strings.Builder
	fmt.Fprintln(&b, "Listening Ports")
	fmt.Fprintln(&b, "")

	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		fmt.Fprintln(&b, "  n/a")
		return portPaneStyle.Render(b.String())
	}

	p := m.filtered[m.cursor]
	if len(p.Ports) == 0 {
		fmt.Fprintln(&b, "  (none)")
	} else {
		for _, port := range p.Ports {
			fmt.Fprintf(&b, "  %d\n", port)
		}
	}

	return portPaneStyle.Render(b.String())
}

func portsForSearch(ports []uint32) string {
	if len(ports) == 0 {
		return ""
	}
	return strings.Join(portsToStrings(ports), " ")
}

func portsToStrings(ports []uint32) []string {
	parts := make([]string, len(ports))
	for i, port := range ports {
		parts[i] = strconv.FormatUint(uint64(port), 10)
	}
	return parts
}

// Start is the entry point for the TUI.
func Start(filter string) {
	p := tea.NewProgram(InitialModel(filter))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
