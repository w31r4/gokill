package tui

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"gkill/internal/process"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// A message containing the list of processes.
type processesMsg []*process.Item

// A message containing an error.
type errMsg struct{ err error }

// model a struct to hold our application's state
type model struct {
	processes []*process.Item
	filtered  []*process.Item
	cursor    int
	textInput textinput.Model
	err       error
}

// InitialModel returns the initial model for the program
func InitialModel(filter string) model {
	ti := textinput.New()
	ti.Placeholder = "Filter processes"
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case processesMsg:
		m.processes = msg
		m.filtered = m.filterProcesses(m.textInput.Value())
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

func (m *model) filterProcesses(filter string) []*process.Item {
	var filtered []*process.Item
	for _, p := range m.processes {
		// Hide killed processes on new filter
		if p.Status == process.Killed {
			continue
		}
		if filter == "" || strings.Contains(strings.ToLower(p.Executable()), strings.ToLower(filter)) {
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

	var b strings.Builder

	// Header
	count := fmt.Sprintf("(%d/%d)", len(m.filtered), len(m.processes))
	fmt.Fprintf(&b, "Filter processes %s: %s\n\n", faintStyle.Render(count), m.textInput.View())

	// No results
	if len(m.filtered) == 0 {
		fmt.Fprintln(&b, "  No results...")
		// Footer
		help := "  "
		if m.textInput.Focused() {
			help += faintStyle.Render("enter/esc to exit filter")
		} else {
			help += faintStyle.Render("q: quit, /: filter, p: pause, r: resume, enter: kill, ctrl+r: refresh")
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
		line := fmt.Sprintf("[%s] %-20s %-10s %d", status, p.Executable(), p.User, p.Pid())

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
		help.WriteString(faintStyle.Render(" enter/esc to exit filter"))
	} else {
		help.WriteString(faintStyle.Render(" /: find • ctrl+r: refresh • r: resume • p: pause • enter: kill • q: quit"))
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
