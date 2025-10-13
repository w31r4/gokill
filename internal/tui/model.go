package tui

import (
	"fmt"
	"os"
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
	ti.Placeholder = "Search processes"
	ti.CharLimit = 156
	ti.Width = 20
	ti.SetValue(filter)

	return model{
		textInput: ti,
	}
}

func (m model) Init() tea.Cmd {
	return getProcesses
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
		return m, nil

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
				return m, sendSignal(p.Pid(), syscall.SIGTERM)
			}
		case "p":
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				p.Status = process.Paused
				return m, sendSignal(p.Pid(), syscall.SIGSTOP)
			}
		case "r":
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				if p.Status == process.Paused {
					p.Status = process.Alive
					return m, sendSignal(p.Pid(), syscall.SIGCONT)
				}
			}
		case "i":
			if len(m.filtered) > 0 {
				m.showDetails = true
				p := m.filtered[m.cursor]
				return m, getProcessDetails(p.Pid())
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
// We combine executable and PID to allow searching on both.
func (s fuzzyProcessSource) String(i int) string {
	p := s.processes[i]
	return fmt.Sprintf("%s %d", p.Executable(), p.Pid())
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
	docStyle      = lipgloss.NewStyle().Margin(1, 2)
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255"))
	faintStyle    = lipgloss.NewStyle().Faint(true)
	killingStyle  = lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("9"))
	pausedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
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

	var b strings.Builder

	// Header
	count := fmt.Sprintf("(%d/%d)", len(m.filtered), len(m.processes))
	fmt.Fprintf(&b, "Search processes %s: %s\n\n", faintStyle.Render(count), m.textInput.View())

	// No results
	if len(m.filtered) == 0 {
		fmt.Fprintln(&b, "  No results...")
		// Footer
		help := "  "
		if m.textInput.Focused() {
			help += faintStyle.Render("enter/esc to exit search")
		} else {
			help += faintStyle.Render("q: quit, /: search, i: info, p: pause, r: resume, enter: kill, ctrl+r: refresh")
		}
		fmt.Fprint(&b, "\n"+help)

		return docStyle.Render(b.String())
	}

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
		line := fmt.Sprintf("[%s] %-20s %-8s %-10s %d", status, p.Executable(), p.StartTime, p.User, p.Pid())

		if i == m.cursor {
			switch p.Status {
			case process.Killed:
				line = killingStyle.Render(line)
			case process.Paused:
				line = pausedStyle.Render(line)
			}
			fmt.Fprintln(&b, selectedStyle.Render("❯ "+line))
		} else {
			switch p.Status {
			case process.Killed:
				line = killingStyle.Render(line)
			case process.Paused:
				line = pausedStyle.Render(line)
			}
			fmt.Fprintln(&b, "  "+faintStyle.Render(line))
		}
	}

	// Footer
	var help strings.Builder
	if m.textInput.Focused() {
		help.WriteString(faintStyle.Render(" enter/esc to exit search"))
	} else {
		help.WriteString(faintStyle.Render(" /: search • i: info • ctrl+r: refresh • r: resume • p: pause • enter: kill • q: quit"))
	}
	fmt.Fprint(&b, "\n"+help.String())

	return docStyle.Render(b.String())
}

// Start is the entry point for the TUI.
func Start(filter string) {
	p := tea.NewProgram(InitialModel(filter))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
