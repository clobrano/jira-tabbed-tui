package actions

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CopyItem is one piece of information the user can copy to the clipboard.
type CopyItem struct {
	Label  string
	Value  string
	Hotkey string // optional extra key that copies this item immediately (e.g. "u" for the URL)
}

// CopyConfirmedMsg is sent when the user confirms what to copy.
type CopyConfirmedMsg struct {
	Text   string
	Labels []string // labels of the copied items, for the status line
}

// CopyCancelledMsg is sent when the user cancels.
type CopyCancelledMsg struct{}

// copySeparators are the joiners cycled with Tab when copying several items.
var copySeparators = []struct{ name, sep string }{
	{"space", " "},
	{"newline", "\n"},
	{"tab", "\t"},
}

var (
	copyOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#5555ff")).
				Padding(1, 2).
				Background(lipgloss.Color("#111111"))

	copyTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).Bold(true)

	copyCursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#222255")).
			Foreground(lipgloss.Color("#ffffff")).Bold(true)

	copyItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#dddddd"))

	copyLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	copyCheckedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00cc44"))

	copyHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))
)

// CopyModel is the overlay for choosing which fields to copy to the clipboard.
//
// Digits 1–9 (or an item's Hotkey) copy that item immediately; Space marks several items, which
// Enter copies in list order joined by the chosen separator. With nothing
// marked, Enter copies the highlighted item.
type CopyModel struct {
	title   string
	items   []CopyItem
	checked []bool
	cursor  int
	sepIdx  int
	width   int
	height  int
}

func NewCopyModel(title string, items []CopyItem) CopyModel {
	return CopyModel{title: title, items: items, checked: make([]bool, len(items))}
}

func (m CopyModel) SetSize(w, h int) CopyModel {
	m.width = w
	m.height = h
	return m
}

// Selection returns the text and labels that Enter would copy.
func (m CopyModel) Selection() (string, []string) {
	var values, labels []string
	for i, it := range m.items {
		if m.checked[i] {
			values = append(values, it.Value)
			labels = append(labels, it.Label)
		}
	}
	if len(values) == 0 && m.cursor < len(m.items) {
		it := m.items[m.cursor]
		return it.Value, []string{it.Label}
	}
	return strings.Join(values, copySeparators[m.sepIdx].sep), labels
}

func (m CopyModel) Update(msg tea.Msg) (CopyModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch s := key.String(); s {
	case "esc", "q":
		return m, func() tea.Msg { return CopyCancelledMsg{} }

	case "j", "down":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case " ", "x":
		if m.cursor < len(m.items) {
			m.checked = append([]bool(nil), m.checked...)
			m.checked[m.cursor] = !m.checked[m.cursor]
		}

	case "a":
		// Toggle all: mark everything unless everything is already marked.
		all := true
		for _, c := range m.checked {
			all = all && c
		}
		m.checked = make([]bool, len(m.items))
		for i := range m.checked {
			m.checked[i] = !all
		}

	case "tab":
		m.sepIdx = (m.sepIdx + 1) % len(copySeparators)

	case "enter":
		text, labels := m.Selection()
		if len(labels) == 0 {
			return m, nil
		}
		return m, func() tea.Msg { return CopyConfirmedMsg{Text: text, Labels: labels} }

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		n := int(s[0] - '1')
		if n < len(m.items) {
			return m, copyItemCmd(m.items[n])
		}

	default:
		for _, it := range m.items {
			if it.Hotkey != "" && it.Hotkey == s {
				return m, copyItemCmd(it)
			}
		}
	}
	return m, nil
}

func copyItemCmd(it CopyItem) tea.Cmd {
	return func() tea.Msg { return CopyConfirmedMsg{Text: it.Value, Labels: []string{it.Label}} }
}

func (m CopyModel) View() string {
	overlayW := 72
	if m.width > 0 && overlayW > m.width-4 {
		overlayW = m.width - 4
	}
	innerW := overlayW - 4

	labelW := 0
	for _, it := range m.items {
		if w := lipgloss.Width(it.Label); w > labelW {
			labelW = w
		}
	}
	if labelW > innerW/3 {
		labelW = innerW / 3
	}
	// "N. [x] " prefix + label column + two spaces.
	valueW := innerW - 7 - labelW - 2
	if valueW < 5 {
		valueW = 5
	}

	// Reserve rows for: title(2) + blank+hint(3, wraps) + border/padding(4).
	pageH := m.height - 9
	if pageH < 3 {
		pageH = 3
	}
	start := 0
	if m.cursor >= pageH {
		start = m.cursor - pageH + 1
	}
	end := start + pageH
	if end > len(m.items) {
		end = len(m.items)
	}

	var sb strings.Builder
	sb.WriteString(copyTitleStyle.Render(m.title) + "\n\n")

	for i := start; i < end; i++ {
		it := m.items[i]
		num := "  "
		if it.Hotkey != "" {
			num = it.Hotkey + "."
		} else if i < 9 {
			num = fmt.Sprintf("%d.", i+1)
		}
		box := "[ ]"
		if m.checked[i] {
			box = "[x]"
		}
		value := oneLine(it.Value)
		if value == "" {
			value = "—"
		}
		value = truncate(value, valueW)
		label := truncate(it.Label, labelW)
		label += strings.Repeat(" ", labelW-lipgloss.Width(label))

		if i == m.cursor {
			line := fmt.Sprintf("%s %s %s  %s", num, box, label, value)
			sb.WriteString(copyCursorStyle.Width(innerW).Render(line) + "\n")
		} else {
			if m.checked[i] {
				box = copyCheckedStyle.Render(box)
			}
			sb.WriteString(copyItemStyle.Render(num+" ") + box + " " +
				copyLabelStyle.Render(label) + "  " + copyItemStyle.Render(value) + "\n")
		}
	}
	if len(m.items) > pageH {
		sb.WriteString(copyHintStyle.Render(fmt.Sprintf("  %d/%d", m.cursor+1, len(m.items))) + "\n")
	}

	sb.WriteString("\n" + copyHintStyle.Render(fmt.Sprintf(
		"1-9/u copy item  •  Space mark  •  a mark all  •  Tab join: %s  •  Enter copy  •  Esc cancel",
		copySeparators[m.sepIdx].name)))

	overlay := copyOverlayStyle.Width(overlayW).Render(sb.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

// oneLine collapses a multi-line value for display in a single row.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	if len(r) > w {
		r = r[:w]
	}
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
