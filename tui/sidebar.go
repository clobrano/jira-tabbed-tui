package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
)

var (
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("#444444")).
			Padding(0, 1)

	sidebarLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888")).
				Bold(true)

	sidebarValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#dddddd"))
)

// Sidebar renders the configured fields panel on the right side of the detail view.
type Sidebar struct {
	fields []config.SidebarField
	width  int
	height int // rows available; 0 = unlimited
}

func NewSidebar(fields []config.SidebarField) Sidebar {
	return Sidebar{fields: fields}
}

func (s Sidebar) SetWidth(w int) Sidebar {
	s.width = w
	return s
}

// SetHeight limits the panel to h rows; fields that don't fit are summarised
// as "… N more fields".
func (s Sidebar) SetHeight(h int) Sidebar {
	s.height = h
	return s
}

func (s Sidebar) View(detail model.IssueDetail) string {
	if s.width < 5 {
		return ""
	}
	inner := s.width - 2 // sidebarStyle pads one column on each side
	valueW := inner - 2  // values are indented by two spaces

	type row struct {
		text  string
		field int // index of the field the row belongs to
	}
	rows := []row{{sidebarLabelStyle.Render("── Fields ──"), -1}}
	for i, sf := range s.fields {
		label := sf.Label
		if label == "" {
			label = fieldDisplayLabel(sf.Field)
		}
		for _, l := range wrapText(label+":", inner) {
			rows = append(rows, row{sidebarLabelStyle.Render(l), i})
		}
		value := fieldValue(detail, sf.Field)
		if value == "" {
			value = "—"
		}
		for _, l := range wrapValue(value, valueW) {
			rows = append(rows, row{sidebarValueStyle.Render("  " + l), i})
		}
	}

	// Too tall for the panel: stop at the last field that fits whole.
	if s.height > 0 && len(rows) > s.height {
		cut := s.height - 1
		for cut > 1 && rows[cut-1].field == rows[cut].field {
			cut--
		}
		hidden := len(s.fields) - rows[cut].field
		rows = append(rows[:cut:cut], row{statusIdleStyle.Render(fmt.Sprintf("… %d more fields", hidden)), -1})
	}

	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = r.text
	}
	return sidebarStyle.Width(s.width).Render(strings.Join(lines, "\n"))
}

// wrapValue wraps a field value to width w. Comma-separated lists (labels,
// components, versions, …) break between items, so an item is only split
// when it is longer than a whole line.
func wrapValue(value string, w int) []string {
	if !strings.Contains(value, ", ") {
		return wrapText(value, w)
	}
	items := strings.Split(value, ", ")
	for i := range items[:len(items)-1] {
		items[i] += ","
	}
	return packTokens(items, w)
}

// wrapText word-wraps s to width w, breaking words longer than a line.
func wrapText(s string, w int) []string {
	return packTokens(strings.Fields(s), w)
}

// packTokens fills lines of width w with space-separated tokens. A token
// wider than w is word-wrapped on its own lines (or hard-broken if it is a
// single long word).
func packTokens(tokens []string, w int) []string {
	if w < 1 {
		w = 1
	}
	var lines []string
	cur := ""
	flush := func() {
		if cur != "" {
			lines = append(lines, cur)
			cur = ""
		}
	}
	for _, t := range tokens {
		tw := lipgloss.Width(t)
		switch {
		case cur != "" && lipgloss.Width(cur)+1+tw <= w:
			cur += " " + t
		case tw <= w:
			flush()
			cur = t
		case strings.Contains(t, " "):
			flush()
			lines = append(lines, packTokens(strings.Fields(t), w)...)
		default:
			flush()
			r := []rune(t)
			for len(r) > w {
				lines = append(lines, string(r[:w]))
				r = r[w:]
			}
			cur = string(r)
		}
	}
	flush()
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

func fieldDisplayLabel(id string) string {
	labels := map[string]string{
		"assignee":  "Assignee",
		"reporter":  "Reporter",
		"labels":    "Labels",
		"duedate":   "Due Date",
		"priority":  "Priority",
		"status":    "Status",
		"issuetype": "Type",
		"created":   "Created",
		"updated":   "Updated",
	}
	if l, ok := labels[id]; ok {
		return l
	}
	return id
}

func fieldValue(detail model.IssueDetail, id string) string {
	val, ok := detail.Fields[id]
	if !ok {
		return ""
	}
	return formatFieldValue(val)
}

func formatFieldValue(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case float64:
		if v == float64(int(v)) {
			return fmt.Sprintf("%d", int(v))
		}
		return fmt.Sprintf("%.2f", v)
	case map[string]any:
		for _, key := range []string{"name", "displayName", "value", "key"} {
			if s, ok := v[key].(string); ok && s != "" {
				return s
			}
		}
		return ""
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			switch iv := item.(type) {
			case string:
				parts = append(parts, iv)
			case map[string]any:
				// Jira object arrays (components, fixVersions, versions, …)
				// carry the human-readable label in one of these keys.
				for _, key := range []string{"name", "displayName", "value", "key"} {
					if s, ok := iv[key].(string); ok && s != "" {
						parts = append(parts, s)
						break
					}
				}
			}
		}
		return strings.Join(parts, ", ")
	case bool:
		if v {
			return "Yes"
		}
		return "No"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}
