package actions

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AddTabSubmittedMsg is sent when the user confirms the new tab form.
type AddTabSubmittedMsg struct {
	Name string
	JQL  string
}

// AddTabCancelledMsg is sent when the user presses Esc.
type AddTabCancelledMsg struct{}

var addTabOverlayStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#5555ff")).
	Padding(1, 2).
	Background(lipgloss.Color("#111111"))

type addTabStep int

const (
	addTabStepName addTabStep = iota
	addTabStepJQL
)

// AddTabModel is the two-step overlay for creating a new tab.
type AddTabModel struct {
	step   addTabStep
	name   textinput.Model
	jql    textinput.Model
	width  int
	height int
}

func NewAddTabModel() AddTabModel {
	name := textinput.New()
	name.Placeholder = "My Tab"
	name.CharLimit = 64
	name.Width = 40
	name.Focus()

	jql := textinput.New()
	jql.Placeholder = `project = PROJ AND status != Done`
	jql.CharLimit = 512
	jql.Width = 40

	return AddTabModel{step: addTabStepName, name: name, jql: jql}
}

func (m AddTabModel) SetSize(w, h int) AddTabModel {
	m.width = w
	m.height = h
	inputW := w - 12
	if inputW < 20 {
		inputW = 20
	}
	m.name.Width = inputW
	m.jql.Width = inputW
	return m
}

func (m AddTabModel) Update(msg tea.Msg) (AddTabModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m.updateInputs(msg)
	}

	switch key.String() {
	case "esc":
		return m, func() tea.Msg { return AddTabCancelledMsg{} }

	case "enter", "tab":
		switch m.step {
		case addTabStepName:
			if m.name.Value() != "" {
				m.step = addTabStepJQL
				m.name.Blur()
				m.jql.Focus()
			}
		case addTabStepJQL:
			if m.jql.Value() != "" && m.name.Value() != "" {
				n, q := m.name.Value(), m.jql.Value()
				return m, func() tea.Msg {
					return AddTabSubmittedMsg{Name: n, JQL: q}
				}
			}
		}
		return m, nil
	}

	return m.updateInputs(msg)
}

func (m AddTabModel) updateInputs(msg tea.Msg) (AddTabModel, tea.Cmd) {
	var cmd tea.Cmd
	switch m.step {
	case addTabStepName:
		m.name, cmd = m.name.Update(msg)
	case addTabStepJQL:
		m.jql, cmd = m.jql.Update(msg)
	}
	return m, cmd
}

func (m AddTabModel) View() string {
	overlayW := 56
	if m.width > 0 && overlayW > m.width-4 {
		overlayW = m.width - 4
	}

	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true).
		Render("Add Tab")
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render("Enter to advance · Esc to cancel")

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaaa"))
	activeLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#5555ff")).Bold(true)

	nameLabel := labelStyle.Render("Name:")
	jqlLabel := labelStyle.Render("JQL: ")
	if m.step == addTabStepName {
		nameLabel = activeLabel.Render("Name:")
	} else {
		jqlLabel = activeLabel.Render("JQL: ")
	}

	content := title + "\n\n" +
		nameLabel + "\n" + m.name.View() + "\n\n" +
		jqlLabel + "\n" + m.jql.View() + "\n\n" +
		hint

	overlay := addTabOverlayStyle.Width(overlayW).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}
