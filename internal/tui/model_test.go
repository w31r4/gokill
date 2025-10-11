package tui

import (
	"testing"

	"gkill/internal/process"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdate(t *testing.T) {
	m := InitialModel("")
	m.processes = []*process.Item{
		process.NewItem(1, "foo", "test"),
		process.NewItem(2, "bar", "test"),
		process.NewItem(3, "baz", "test"),
	}
	m.filtered = m.processes

	// Test moving down
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = newModel.(model)
	if m.cursor != 1 {
		t.Errorf("cursor should be 1, but got %d", m.cursor)
	}

	// Test moving up
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = newModel.(model)
	if m.cursor != 0 {
		t.Errorf("cursor should be 0, but got %d", m.cursor)
	}
}

func TestFilterProcesses(t *testing.T) {
	m := model{
		processes: []*process.Item{
			process.NewItem(1, "foo", "test"),
			process.NewItem(2, "bar", "test"),
			process.NewItem(3, "foobar", "test"),
		},
	}

	filtered := m.filterProcesses("foo")
	if len(filtered) != 2 {
		t.Errorf("expected 2 processes, but got %d", len(filtered))
	}

	filtered = m.filterProcesses("bar")
	if len(filtered) != 2 {
		t.Errorf("expected 2 processes, but got %d", len(filtered))
	}

	filtered = m.filterProcesses("baz")
	if len(filtered) != 0 {
		t.Errorf("expected 0 processes, but got %d", len(filtered))
	}
}
