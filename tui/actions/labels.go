package actions

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LabelsSubmittedMsg is sent when the user confirms the labels input.
type LabelsSubmittedMsg struct {
	IssueKey string
	Labels   []string
}

// LabelsCancelledMsg is sent when the user presses Esc.
type LabelsCancelledMsg struct{}

var labelsOverlayStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#5555ff")).
	Padding(1, 2).
	Background(lipgloss.Color("#111111"))

// LabelsModel is the inline overlay for adding labels.
type LabelsModel struct {
	issueKey string
	input    textinput.Model
	width    int
	height   int
}

func NewLabelsModel(issueKey string) LabelsModel {
	ti := textinput.New()
	ti.Placeholder = "backend, performance, v2"
	ti.CharLimit = 256
	ti.Width = 40
	ti.Focus()
	return LabelsModel{issueKey: issueKey, input: ti}
}

func (m LabelsModel) SetSize(w, h int) LabelsModel {
	m.width = w
	m.height = h
	inputW := 44
	if inputW > w-8 {
		inputW = w - 8
	}
	m.input.Width = inputW - 4
	return m
}

func (m LabelsModel) Update(msg tea.Msg) (LabelsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	switch key.String() {
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if val == "" {
			return m, func() tea.Msg { return LabelsCancelledMsg{} }
		}
		parts := strings.Split(val, ",")
		labels := make([]string, 0, len(parts))
		for _, p := range parts {
			if l := strings.TrimSpace(p); l != "" {
				labels = append(labels, l)
			}
		}
		return m, func() tea.Msg {
			return LabelsSubmittedMsg{IssueKey: m.issueKey, Labels: labels}
		}
	case "esc":
		return m, func() tea.Msg { return LabelsCancelledMsg{} }
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m LabelsModel) View() string {
	overlayW := 52
	if m.width > 0 && overlayW > m.width-4 {
		overlayW = m.width - 4
	}
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true).Render("Add Labels")
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render("Comma-separated — Enter to confirm, Esc to cancel")
	content := title + "\n\n" + m.input.View() + "\n\n" + hint
	overlay := labelsOverlayStyle.Width(overlayW).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}
