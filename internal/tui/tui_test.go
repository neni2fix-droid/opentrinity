package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMaskedInputNeverDisplaysSecret(t *testing.T) {
	m := &model{
		services: Services{},
		view:     viewInput,
		prompt:   "Secret (masked)",
		masked:   true,
		input:    []rune("canary-secret-value"),
	}
	view := m.View()
	if strings.Contains(view, "canary-secret-value") {
		t.Fatal("TUI displayed the secret value")
	}
	if !strings.Contains(view, strings.Repeat("*", len("canary-secret-value"))) {
		t.Fatal("TUI did not render masked input")
	}
}

func TestMenuKeyboardNavigationAndQuit(t *testing.T) {
	m := &model{services: Services{}, view: viewMenu}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	nm := updated.(*model)
	if nm.menuIdx != 1 {
		t.Fatalf("menuIdx=%d want=1", nm.menuIdx)
	}
	updated, cmd := nm.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c did not return a quit command")
	}
	updated.(*model).View()
}
