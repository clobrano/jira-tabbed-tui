package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clobrano/jira-tabbed-tui/model"
)

var (
	tableBaseStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#444444"))

	staleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffaa00")).
			Italic(true)

	errorListStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff4444")).
			Padding(1, 2)

	loadMoreStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true).
			Padding(0, 2)
)

// IssueList renders the five-column issue table for a single tab.
type IssueList struct {
	issues    []model.Issue
	total     int
	loading   bool
	loadingMore bool
	err       error
	stale     bool
	cursor    int
	width     int
	height    int
	spinner   spinner.Model
	tbl       table.Model
}

func NewIssueList() IssueList {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#5555ff"))
	return IssueList{spinner: s, loading: true}
}

func (l IssueList) SetSize(w, h int) IssueList {
	l.width = w
	l.height = h
	l.tbl = l.buildTable()
	return l
}

func (l IssueList) SetIssues(issues []model.Issue, total int, stale bool, err error) IssueList {
	l.issues = issues
	l.total = total
	l.stale = stale
	l.err = err
	l.loading = false
	l.loadingMore = false
	if l.cursor >= len(issues) && len(issues) > 0 {
		l.cursor = len(issues) - 1
	}
	l.tbl = l.buildTable()
	return l
}

func (l IssueList) SetLoading(v bool) IssueList {
	l.loading = v
	l.tbl = l.buildTable()
	return l
}

func (l IssueList) SetLoadingMore(v bool) IssueList {
	l.loadingMore = v
	return l
}

func (l IssueList) MoveUp() IssueList {
	if l.cursor > 0 {
		l.cursor--
		l.tbl.MoveUp(1)
	}
	return l
}

func (l IssueList) MoveDown() IssueList {
	if l.cursor < len(l.issues)-1 {
		l.cursor++
		l.tbl.MoveDown(1)
	}
	return l
}

func (l IssueList) Cursor() int { return l.cursor }

func (l IssueList) SelectedIssue() (model.Issue, bool) {
	if l.cursor < 0 || l.cursor >= len(l.issues) {
		return model.Issue{}, false
	}
	return l.issues[l.cursor], true
}

// AtBottom returns true when the cursor is at the last item.
func (l IssueList) AtBottom() bool {
	return len(l.issues) > 0 && l.cursor == len(l.issues)-1
}

// HasMore returns true when there are more items on the server.
func (l IssueList) HasMore() bool {
	return l.total > len(l.issues)
}

func (l IssueList) SpinnerTick() (IssueList, tea.Cmd) {
	var cmd tea.Cmd
	l.spinner, cmd = l.spinner.Update(l.spinner.Tick())
	return l, cmd
}

func (l IssueList) Init() tea.Cmd {
	return l.spinner.Tick
}

func (l IssueList) buildTable() table.Model {
	if l.width == 0 {
		return table.Model{}
	}
	const (
		keyW  = 12
		typeW = 10
		priW  = 10
		dueW  = 12
	)
	borders := 3 // rough estimate for borders
	summaryW := l.width - keyW - typeW - priW - dueW - borders
	if summaryW < 10 {
		summaryW = 10
	}

	cols := []table.Column{
		{Title: "Key", Width: keyW},
		{Title: "Type", Width: typeW},
		{Title: "Summary", Width: summaryW},
		{Title: "Priority", Width: priW},
		{Title: "Due Date", Width: dueW},
	}

	rows := make([]table.Row, 0, len(l.issues))
	for _, iss := range l.issues {
		summary := iss.Summary
		if len(summary) > summaryW-2 {
			summary = summary[:summaryW-2] + "…"
		}
		rows = append(rows, table.Row{iss.Key, iss.Type, summary, iss.Priority, iss.DueDate})
	}

	tableHeight := l.height - 4
	if tableHeight < 3 {
		tableHeight = 3
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(tableHeight),
	)
	ts := table.DefaultStyles()
	ts.Header = ts.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#444444")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("#cccccc"))
	ts.Selected = ts.Selected.
		Foreground(lipgloss.Color("#ffffff")).
		Background(lipgloss.Color("#3333aa")).
		Bold(true)
	t.SetStyles(ts)
	// Sync cursor position.
	if l.cursor >= 0 && l.cursor < len(rows) {
		t.SetCursor(l.cursor)
	}
	return t
}

func (l IssueList) View() string {
	if l.loading {
		return fmt.Sprintf("\n  %s Loading…", l.spinner.View())
	}

	var sb strings.Builder

	if l.err != nil && l.stale {
		sb.WriteString(staleStyle.Render("  ⚠ Possibly out of date — last fetch failed: " + l.err.Error()))
		sb.WriteString("\n")
	} else if l.err != nil {
		return errorListStyle.Render("Error: " + MapCLIError(l.err.Error()))
	}

	if len(l.issues) == 0 {
		sb.WriteString(statusIdleStyle.Padding(1, 2).Render("No issues found."))
		return sb.String()
	}

	sb.WriteString(tableBaseStyle.Render(l.tbl.View()))

	if l.loadingMore {
		sb.WriteString("\n" + loadMoreStyle.Render("  Loading more…"))
	} else if l.HasMore() {
		sb.WriteString("\n" + loadMoreStyle.Render(
			fmt.Sprintf("  %d of %d — press j at bottom to load more", len(l.issues), l.total)))
	}

	return sb.String()
}
