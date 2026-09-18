package actions

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// EditJQLSubmittedMsg is sent when the user confirms the edited JQL.
type EditJQLSubmittedMsg struct {
	JQL string
}

// EditJQLCancelledMsg is sent when the user presses Esc.
type EditJQLCancelledMsg struct{}

var editJQLOverlayStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#5555ff")).
	Padding(1, 2).
	Background(lipgloss.Color("#111111"))

// EditJQLModel is the overlay for editing a tab's JQL at runtime.
type EditJQLModel struct {
	input    textinput.Model
	tabName  string
	width    int
	height   int
}

func NewEditJQLModel(tabName, currentJQL string) EditJQLModel {
	ti := textinput.New()
	ti.SetValue(currentJQL)
	ti.CharLimit = 512
	ti.Width = 50
	ti.Focus()
	// Place cursor at end.
	ti.CursorEnd()
	return EditJQLModel{input: ti, tabName: tabName}
}

func (m EditJQLModel) SetSize(w, h int) EditJQLModel {
	m.width = w
	m.height = h
	inputW := w - 12
	if inputW < 20 {
		inputW = 20
	}
	m.input.Width = inputW
	return m
}

func (m EditJQLModel) Update(msg tea.Msg) (EditJQLModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	switch key.String() {
	case "enter":
		jql := m.input.Value()
		if jql != "" {
			return m, func() tea.Msg { return EditJQLSubmittedMsg{JQL: jql} }
		}
	case "esc":
		return m, func() tea.Msg { return EditJQLCancelledMsg{} }
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m EditJQLModel) View() string {
	overlayW := 60
	if m.width > 0 && overlayW > m.width-4 {
		overlayW = m.width - 4
	}

	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true).
		Render("Edit JQL — " + m.tabName)
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render("Enter to apply (temporary) · Esc to cancel")

	content := title + "\n\n" + m.input.View() + "\n\n" + hint
	overlay := editJQLOverlayStyle.Width(overlayW).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}
