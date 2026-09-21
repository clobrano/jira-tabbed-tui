package actions

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
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

	optionPickerFilterPrompt = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#5555ff")).Bold(true)
)

// OptionPickerModel is a generic list-picker overlay for field values with search.
type OptionPickerModel struct {
	fieldID      string
	fieldName    string
	issueKey     string
	allOptions   []string // full unfiltered list
	filtered     []string // currently visible (matches filter)
	currentValue string
	cursor       int
	filter       textinput.Model
	width        int
	height       int
}

func NewOptionPickerModel(issueKey, fieldID, fieldName, currentValue string, options []string) OptionPickerModel {
	ti := textinput.New()
	ti.Placeholder = "search…"
	ti.CharLimit = 64
	ti.Focus()

	m := OptionPickerModel{
		issueKey:     issueKey,
		fieldID:      fieldID,
		fieldName:    fieldName,
		currentValue: currentValue,
		allOptions:   options,
		filtered:     options,
		filter:       ti,
	}
	m.cursor = m.indexOfCurrent()
	return m
}

func (m OptionPickerModel) SetSize(w, h int) OptionPickerModel {
	m.width = w
	m.height = h
	innerW := w - 8
	if innerW < 20 {
		innerW = 20
	}
	m.filter.Width = innerW - 4
	return m
}

func (m OptionPickerModel) indexOfCurrent() int {
	for i, o := range m.filtered {
		if strings.EqualFold(o, m.currentValue) {
			return i
		}
	}
	return 0
}

func (m OptionPickerModel) applyFilter(q string) OptionPickerModel {
	if q == "" {
		m.filtered = m.allOptions
	} else {
		q = strings.ToLower(q)
		out := make([]string, 0, len(m.allOptions))
		for _, o := range m.allOptions {
			if strings.Contains(strings.ToLower(o), q) {
				out = append(out, o)
			}
		}
		m.filtered = out
	}
	m.cursor = 0
	return m
}

func (m OptionPickerModel) Update(msg tea.Msg) (OptionPickerModel, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		return m, cmd
	}

	switch key.String() {
	case "esc":
		return m, func() tea.Msg { return OptionPickCancelledMsg{} }

	case "enter":
		if m.cursor < len(m.filtered) {
			v := m.filtered[m.cursor]
			fid, fname, ikey := m.fieldID, m.fieldName, m.issueKey
			return m, func() tea.Msg {
				return OptionPickedMsg{FieldID: fid, FieldName: fname, IssueKey: ikey, Value: v}
			}
		}

	case "j", "down":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	default:
		prev := m.filter.Value()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		if m.filter.Value() != prev {
			m = m.applyFilter(m.filter.Value())
		}
		return m, cmd
	}

	return m, nil
}

func (m OptionPickerModel) View() string {
	overlayW := 52
	if m.width > 0 && overlayW > m.width-4 {
		overlayW = m.width - 4
	}
	innerW := overlayW - 4

	// Reserve rows for: title(1) + blank(1) + filter(1) + blank(1) + hint(1) + blank(1) + border+padding(4) = 10
	pageH := m.height - 10
	if pageH < 3 {
		pageH = 3
	}

	var sb strings.Builder
	sb.WriteString(optionPickerTitleStyle.Render("Edit: "+m.fieldName) + "\n\n")
	sb.WriteString(optionPickerFilterPrompt.Render("/") + " " + m.filter.View() + "\n\n")

	if len(m.filtered) == 0 {
		sb.WriteString(optionPickerHintStyle.Render("  no matches") + "\n")
	} else {
		// Scroll window: keep cursor visible.
		start := 0
		if m.cursor >= pageH {
			start = m.cursor - pageH + 1
		}
		end := start + pageH
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		for i, opt := range m.filtered[start:end] {
			abs := start + i
			var label string
			if strings.EqualFold(opt, m.currentValue) {
				label = optionPickerCurrentStyle.Render("● " + opt)
			} else {
				label = optionPickerItemStyle.Render("  " + opt)
			}
			if abs == m.cursor {
				label = optionPickerCursorStyle.Width(innerW).Render(label)
			}
			sb.WriteString(label + "\n")
		}
		if len(m.filtered) > pageH {
			sb.WriteString(optionPickerHintStyle.Render(
				fmt.Sprintf("  %d/%d", m.cursor+1, len(m.filtered))) + "\n")
		}
	}

	sb.WriteString("\n" + optionPickerHintStyle.Render("type filter  •  j/k navigate  •  Enter select  •  Esc cancel"))

	overlay := optionPickerOverlayStyle.Width(overlayW).Render(sb.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}
