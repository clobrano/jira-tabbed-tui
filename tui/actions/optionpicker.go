package actions

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// OptionPickedMsg is sent when the user selects an option.
type OptionPickedMsg struct {
	FieldID   string
	FieldName string
	IssueKey  string
	Value     string
}

// OptionPickCancelledMsg is sent when the user cancels.
type OptionPickCancelledMsg struct{}

// FieldTextSubmittedMsg is sent when the user confirms a free-text field edit.
type FieldTextSubmittedMsg struct {
	FieldID   string
	FieldName string
	IssueKey  string
	Value     string
}

// FieldTextCancelledMsg is sent when the user cancels free-text editing.
type FieldTextCancelledMsg struct{}

var (
	optionPickerOverlayStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("#5555ff")).
					Padding(1, 2).
					Background(lipgloss.Color("#111111"))

	optionPickerTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ffffff")).Bold(true)

	optionPickerCursorStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#222255")).
				Foreground(lipgloss.Color("#ffffff")).Bold(true)

	optionPickerItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#dddddd"))

	optionPickerCurrentStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#00cc44"))

	optionPickerHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888"))
)

// OptionPickerModel is a generic list-picker overlay for field values.
type OptionPickerModel struct {
	fieldID      string
	fieldName    string
	issueKey     string
	options      []string
	currentValue string
	cursor       int
	width        int
	height       int
}

func NewOptionPickerModel(issueKey, fieldID, fieldName, currentValue string, options []string) OptionPickerModel {
	m := OptionPickerModel{
		issueKey:     issueKey,
		fieldID:      fieldID,
		fieldName:    fieldName,
		currentValue: currentValue,
		options:      options,
	}
	// Pre-position cursor on the current value.
	for i, o := range options {
		if strings.EqualFold(o, currentValue) {
			m.cursor = i
			break
		}
	}
	return m
}

func (m OptionPickerModel) SetSize(w, h int) OptionPickerModel {
	m.width = w
	m.height = h
	return m
}

func (m OptionPickerModel) Update(msg tea.Msg) (OptionPickerModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "j", "down":
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if m.cursor < len(m.options) {
			v := m.options[m.cursor]
			return m, func() tea.Msg {
				return OptionPickedMsg{
					FieldID: m.fieldID, FieldName: m.fieldName,
					IssueKey: m.issueKey, Value: v,
				}
			}
		}
	case "esc", "q":
		return m, func() tea.Msg { return OptionPickCancelledMsg{} }
	}
	return m, nil
}

func (m OptionPickerModel) View() string {
	overlayW := 48
	if m.width > 0 && overlayW > m.width-4 {
		overlayW = m.width - 4
	}
	innerW := overlayW - 4

	var sb strings.Builder
	sb.WriteString(optionPickerTitleStyle.Render("Edit: "+m.fieldName) + "\n\n")

	for i, opt := range m.options {
		label := opt
		if strings.EqualFold(opt, m.currentValue) {
			label = optionPickerCurrentStyle.Render("● " + opt)
		} else {
			label = optionPickerItemStyle.Render("  " + opt)
		}
		if i == m.cursor {
			label = optionPickerCursorStyle.Width(innerW).Render(label)
		}
		sb.WriteString(label + "\n")
	}

	sb.WriteString("\n" + optionPickerHintStyle.Render("j/k navigate  •  Enter select  •  Esc cancel"))

	overlay := optionPickerOverlayStyle.Width(overlayW).Render(sb.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}
