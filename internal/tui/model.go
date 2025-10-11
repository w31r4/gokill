package tui

import (
	"fmt"
	"os"
	"strings"

	"gkill/internal/process"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// A message containing the list of processes.
type processesMsg []process.Process

// A message containing an error.
type errMsg struct{ err error }

// model a struct to hold our application's state
type model struct {
	processes []process.Process
	filtered  []process.Process
	cursor    int
	textInput textinput.Model
	err       error
}

// InitialModel returns the initial model for the program
func InitialModel(filter string) model {
	ti := textinput.New()
	ti.Placeholder = "Filter processes"
	ti.Focus()
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
		m.filtered = msg
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
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
				return m, m.killProcess(m.filtered[m.cursor].Pid())
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

func (m *model) filterProcesses(filter string) []process.Process {
	if filter == "" {
		return m.processes
	}

	var filtered []process.Process
	for _, p := range m.processes {
		if strings.Contains(strings.ToLower(p.Executable()), strings.ToLower(filter)) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func (m *model) killProcess(pid int) tea.Cmd {
	return func() tea.Msg {
		if err := process.KillProcess(pid); err != nil {
			return errMsg{err}
		}
		// After killing, we should refresh the process list.
		return getProcesses()
	}
}

var (
	docStyle      = lipgloss.NewStyle().Margin(1, 2)
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255"))
	faintStyle    = lipgloss.NewStyle().Faint(true)
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
		line := fmt.Sprintf("%-30s %d", p.Executable(), p.Pid())

		if i == m.cursor {
			fmt.Fprintln(&b, selectedStyle.Render("❯ "+line))
		} else {
			fmt.Fprintln(&b, "  "+faintStyle.Render(line))
		}
	}

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
