// Package tui is a small Bubble Tea shell over the same services used by the CLI.
// It deliberately has no independent business logic and never reveals secrets.
package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Provider struct {
	ID      int64
	Name    string
	APIBase string
}

type Model struct {
	ID      int64
	Label   string
	Slot    string
	Enabled bool
}

// Services contains the operations the TUI may perform.
type Services struct {
	Status        func() string
	Providers     func() ([]Provider, error)
	AddProvider   func(name, apiBase string) error
	AddCredential func(providerID int64, label, secret string) error
	Models        func() ([]Model, error)
	SetDeployment func(slot string, modelID int64) error
	Apply         func() error
	Rollback      func() error
}

type viewState int

const (
	viewMenu viewState = iota
	viewInput
	viewPicker
	viewConfirm
	viewMessage
)

type model struct {
	services Services
	view     viewState
	menuIdx  int
	message  string
	prompt   string
	masked   bool
	input    []rune
	pending  string
	options  []string
	optionID []int64
	pickIdx  int
	errors   string

	label             string
	pendingProvider   int64
	deploymentSlot    string
	deploymentModelID int64
}

var menu = []string{
	"Status overview",
	"Add provider",
	"Add credential (masked)",
	"Assign deployment slot",
	"Validate and apply",
	"Rollback to last known good",
	"Help",
	"Quit",
}

// Run starts the interactive program.
func Run(services Services) error {
	p := tea.NewProgram(&model{services: services, view: viewMenu}, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	switch m.view {
	case viewMenu:
		switch key.String() {
		case "up", "k":
			if m.menuIdx > 0 {
				m.menuIdx--
			}
		case "down", "j":
			if m.menuIdx < len(menu)-1 {
				m.menuIdx++
			}
		case "enter":
			return m.selectMenu()
		case "1", "2", "3", "4", "5", "6", "7", "8":
			n, _ := strconv.Atoi(key.String())
			m.menuIdx = n - 1
			return m.selectMenu()
		}
	case viewInput:
		switch key.Type {
		case tea.KeyEsc:
			m.view, m.input, m.errors = viewMenu, nil, ""
		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		case tea.KeyEnter:
			value := strings.TrimSpace(string(m.input))
			m.input = nil
			if value == "" {
				m.errors = "input cannot be empty"
				return m, nil
			}
			return m.submitInput(value)
		default:
			if len(key.Runes) > 0 && key.Type != tea.KeyCtrlC {
				m.input = append(m.input, key.Runes...)
			}
		}
	case viewPicker:
		switch key.String() {
		case "esc":
			m.view, m.errors = viewMenu, ""
		case "up", "k":
			if m.pickIdx > 0 {
				m.pickIdx--
			}
		case "down", "j":
			if m.pickIdx < len(m.options)-1 {
				m.pickIdx++
			}
		case "enter":
			if len(m.optionID) == 0 || m.pickIdx >= len(m.optionID) {
				return m, nil
			}
			id := m.optionID[m.pickIdx]
			return m.submitPick(id)
		}
	case viewConfirm:
		switch strings.ToLower(key.String()) {
		case "y":
			return m.submitConfirm(true)
		case "n", "esc":
			m.view, m.errors = viewMenu, ""
		}
	case viewMessage:
		if key.String() == "enter" || key.String() == "esc" {
			m.view, m.errors = viewMenu, ""
		}
	}
	return m, nil
}

func (m *model) selectMenu() (tea.Model, tea.Cmd) {
	m.errors = ""
	switch m.menuIdx {
	case 0:
		m.message = m.services.Status()
		m.view = viewMessage
	case 1:
		m.pending, m.prompt, m.masked, m.view = "provider-base", "API base (e.g. http://127.0.0.1:10001/v1)", false, viewInput
	case 2:
		providers, err := m.services.Providers()
		if err != nil || len(providers) == 0 {
			m.setError("add a provider first")
			return m, nil
		}
		m.options = nil
		m.optionID = nil
		for _, p := range providers {
			m.options = append(m.options, fmt.Sprintf("%d  %s", p.ID, p.Name))
			m.optionID = append(m.optionID, p.ID)
		}
		m.pending, m.pickIdx, m.view = "credential-provider", 0, viewPicker
	case 3:
		models, err := m.services.Models()
		if err != nil || len(models) == 0 {
			m.setError("add a model first")
			return m, nil
		}
		m.options = []string{"Slot A", "Slot B", "Slot C"}
		m.optionID = []int64{0, 1, 2}
		m.pending, m.pickIdx, m.view = "deployment-slot", 0, viewPicker
	case 4:
		m.prompt, m.pending, m.view = "Validate and activate this configuration? [y/N]", "apply", viewConfirm
	case 5:
		m.prompt, m.pending, m.view = "Restore the last known good configuration? [y/N]", "rollback", viewConfirm
	case 6:
		m.message = "Keys: up/down navigate, enter select, esc cancel, ctrl+c quit. Secrets are masked and never shown."
		m.view = viewMessage
	case 7:
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) submitInput(value string) (tea.Model, tea.Cmd) {
	switch m.pending {
	case "provider-base":
		m.pending, m.prompt, m.masked, m.view = "provider-name", "Provider name", false, viewInput
		m.message = value
	case "provider-name":
		if err := m.services.AddProvider(value, m.message); err != nil {
			m.setError(err.Error())
			return m, nil
		}
		m.message = "Provider added."
		m.view = viewMessage
	case "credential-label":
		m.label = value
		m.pending, m.prompt, m.masked, m.view = "credential-secret", "Secret (masked)", true, viewInput
	case "credential-secret":
		if err := m.services.AddCredential(m.pendingProvider, m.label, value); err != nil {
			m.setError("credential could not be saved")
			return m, nil
		}
		m.message = "Credential stored in Windows Credential Manager. Value not displayed."
		m.view = viewMessage
	}
	return m, nil
}

func (m *model) submitPick(id int64) (tea.Model, tea.Cmd) {
	switch m.pending {
	case "credential-provider":
		m.pendingProvider = id
		m.label = ""
		m.pending, m.prompt, m.masked, m.view = "credential-label", "Credential label", false, viewInput
	case "deployment-slot":
		models, err := m.services.Models()
		if err != nil {
			m.setError("model list unavailable")
			return m, nil
		}
		m.options = nil
		m.optionID = nil
		for _, item := range models {
			label := item.Label
			if label == "" {
				label = fmt.Sprintf("model %d", item.ID)
			}
			m.options = append(m.options, fmt.Sprintf("%d  %s", item.ID, label))
			m.optionID = append(m.optionID, item.ID)
		}
		m.deploymentSlot = []string{"A", "B", "C"}[id]
		m.pending = "deployment-model"
		m.pickIdx, m.view = 0, viewPicker
	case "deployment-model":
		m.deploymentModelID = id
		if err := m.services.SetDeployment(m.deploymentSlot, id); err != nil {
			m.setError(err.Error())
			return m, nil
		}
		m.message = fmt.Sprintf("Deployment slot %s updated.", m.deploymentSlot)
		m.view = viewMessage
	}
	return m, nil
}

func (m *model) submitConfirm(yes bool) (tea.Model, tea.Cmd) {
	if !yes {
		m.view = viewMenu
		return m, nil
	}
	if m.pending == "apply" {
		if err := m.services.Apply(); err != nil {
			m.setError("apply failed; active configuration was preserved")
		} else {
			m.message = "Configuration applied."
			m.view = viewMessage
		}
	} else if m.pending == "rollback" {
		if err := m.services.Rollback(); err != nil {
			m.setError("rollback failed")
		} else {
			m.message = "Last known good configuration restored."
			m.view = viewMessage
		}
	}
	return m, nil
}

func (m *model) setError(msg string) {
	m.errors = msg
	m.message = msg
	m.view = viewMessage
}

func (m *model) View() string {
	var b strings.Builder
	b.WriteString("OPENTRINITY — local AI gateway\n\n")
	switch m.view {
	case viewMenu:
		for i, item := range menu {
			cursor := "  "
			if i == m.menuIdx {
				cursor = "> "
			}
			fmt.Fprintf(&b, "%s%d. %s\n", cursor, i+1, item)
		}
	case viewInput:
		b.WriteString(m.prompt + "\n> ")
		if m.masked {
			b.WriteString(strings.Repeat("*", len(m.input)))
		} else {
			b.WriteString(string(m.input))
		}
	case viewPicker:
		b.WriteString(m.promptForPicker() + "\n")
		for i, opt := range m.options {
			cursor := "  "
			if i == m.pickIdx {
				cursor = "> "
			}
			b.WriteString(cursor + opt + "\n")
		}
	case viewConfirm:
		b.WriteString(m.prompt + "\n")
	case viewMessage:
		b.WriteString(m.message + "\n")
	}
	if m.errors != "" {
		b.WriteString("\nError: " + m.errors + "\n")
	}
	b.WriteString("\nup/down navigate · enter select · esc cancel · ctrl+c quit")
	return b.String()
}

func (m *model) promptForPicker() string {
	if m.pending == "credential-provider" {
		return "Select provider:"
	}
	if m.pending == "deployment-slot" {
		return "Select slot:"
	}
	return "Select model:"
}
