package tui

import (
	"testing"

	"github.com/w31r4/gokill/internal/process"

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

func TestTModeEnterFromMainList(t *testing.T) {
	m := InitialModel("")
	m.processes = []*process.Item{
		{Pid: 1, PPid: 0, Executable: "root", User: "test", Status: process.Alive},
		{Pid: 2, PPid: 1, Executable: "child", User: "test", Status: process.Alive},
	}
	m.filtered = m.processes
	m.cursor = 1 // select the second item as root

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = newModel.(model)

	if !m.dep.mode {
		t.Fatalf("expected depMode to be true after pressing T")
	}
	if m.dep.rootPID != 2 {
		t.Fatalf("expected depRootPID to be 2, got %d", m.dep.rootPID)
	}
	if m.dep.cursor != 0 {
		t.Fatalf("expected depCursor to start at 0, got %d", m.dep.cursor)
	}
	if m.dep.expanded == nil {
		t.Fatalf("expected depExpanded to be initialized")
	}
	if st, ok := m.dep.expanded[2]; !ok {
		t.Fatalf("expected depExpanded to contain root pid 2")
	} else {
		if !st.expanded {
			t.Errorf("expected root node to be expanded in depExpanded")
		}
		if st.page != 1 {
			t.Errorf("expected root node page to be 1, got %d", st.page)
		}
	}
}

func TestTModeNavigationAndExit(t *testing.T) {
	m := InitialModel("")
	m.processes = []*process.Item{
		{Pid: 1, PPid: 0, Executable: "root", User: "test", Status: process.Alive},
		{Pid: 2, PPid: 1, Executable: "child", User: "test", Status: process.Alive},
	}
	m.filtered = m.processes
	m.cursor = 0

	// Enter T-mode with the root process selected.
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = newModel.(model)
	if !m.dep.mode {
		t.Fatalf("expected depMode to be true after pressing T")
	}

	// Move the dependency cursor down.
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = newModel.(model)
	if m.dep.cursor == 0 {
		t.Errorf("expected depCursor to move down from 0")
	}

	// Exit T-mode with ESC.
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newModel.(model)
	if m.dep.mode {
		t.Errorf("expected depMode to be false after pressing esc")
	}
	if m.dep.expanded != nil && len(m.dep.expanded) != 0 {
		t.Errorf("expected depExpanded to be cleared after exiting T-mode")
	}
	if m.dep.cursor != 0 {
		t.Errorf("expected depCursor to reset to 0 after exiting T-mode, got %d", m.dep.cursor)
	}
}

func TestFilterProcesses(t *testing.T) {
	m := model{
		processes: []*process.Item{
			process.NewItem(1, "foo", "test", 8080),
			process.NewItem(2, "bar", "test"),
			process.NewItem(3, "foobar", "test", 5432),
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

	filtered = m.filterProcesses("8080")
	if len(filtered) != 1 || filtered[0].Pid != 1 {
		t.Errorf("expected to find process with pid 1 for port search, but got %#v", filtered)
	}
}
