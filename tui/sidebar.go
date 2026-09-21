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
}

func NewSidebar(fields []config.SidebarField) Sidebar {
	return Sidebar{fields: fields}
}

func (s Sidebar) SetWidth(w int) Sidebar {
	s.width = w
	return s
}

func (s Sidebar) View(detail model.IssueDetail) string {
	if s.width < 5 {
		return ""
	}
	labelW := 12
	valueW := s.width - labelW - 4 // borders + padding
	if valueW < 5 {
		valueW = 5
	}

	var rows []string
	rows = append(rows, sidebarLabelStyle.Width(s.width-2).Render("── Fields ──"))

	for _, sf := range s.fields {
		label := sf.Label
		if label == "" {
			label = fieldDisplayLabel(sf.Field)
		}
		value := fieldValue(detail, sf.Field)
		if value == "" {
			value = "—"
		}
		// Wrap long values.
		if len(value) > valueW {
			value = value[:valueW-1] + "…"
		}
		row := fmt.Sprintf("%s\n%s",
			sidebarLabelStyle.Width(s.width-2).Render(label+":"),
			sidebarValueStyle.Width(s.width-2).Render("  "+value),
		)
		rows = append(rows, row)
	}

	content := strings.Join(rows, "\n")
	return sidebarStyle.Width(s.width).Render(content)
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
