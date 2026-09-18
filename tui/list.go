package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
	"github.com/sahilm/fuzzy"
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

	filterBarActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#5555ff")).
				Bold(true)

	filterBarAppliedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#aaaaff"))

	filterHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555")).
			Italic(true)
)

// columnDefaultWidths holds the default width for each known list field.
// Width 0 means the column is flexible (expands to fill remaining space).
var columnDefaultWidths = map[string]int{
	"key":      12,
	"type":     10,
	"summary":  0, // flexible
	"priority": 10,
	"status":   12,
	"duedate":  12,
	"assignee": 20,
	"created":  12,
	"updated":  12,
}

// columnDefaultLabels holds the header label for each known list field.
var columnDefaultLabels = map[string]string{
	"key":      "Key",
	"type":     "Type",
	"summary":  "Summary",
	"priority": "Priority",
	"status":   "Status",
	"duedate":  "Due Date",
	"assignee": "Assignee",
	"created":  "Created",
	"updated":  "Updated",
}

// IssueList renders a configurable-column issue table for a single tab.
type IssueList struct {
	columns      []config.ListColumn
	allIssues    []model.Issue // full unfiltered set (in current sort order)
	issues       []model.Issue // currently displayed (filtered or all)
	filterActive bool
	filterQuery  string
	filterInput  textinput.Model
	sortField    string // "" = server order
	sortAsc      bool
	total        int
	loading      bool
	loadingMore  bool
	err          error
	stale        bool
	cursor       int
	width        int
	height       int
	spinner      spinner.Model
	tbl          table.Model
}

func NewIssueList(columns []config.ListColumn) IssueList {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#5555ff"))
	ti := textinput.New()
	ti.Placeholder = "fuzzy filter…"
	ti.CharLimit = 128
	return IssueList{columns: columns, spinner: s, loading: true, filterInput: ti}
}

func (l IssueList) SetSize(w, h int) IssueList {
	l.width = w
	l.height = h
	l.tbl = l.buildTable()
	return l
}

func (l IssueList) SetIssues(issues []model.Issue, total int, stale bool, err error) IssueList {
	l.total = total
	l.stale = stale
	l.err = err
	l.loading = false
	l.loadingMore = false
	// Re-apply current sort to fresh data.
	if l.sortField != "" {
		l.allIssues = sortIssues(issues, l.sortField, l.sortAsc)
	} else {
		l.allIssues = issues
	}
	if l.filterQuery != "" {
		l.issues = applyFuzzyFilter(l.filterQuery, l.allIssues)
	} else {
		l.issues = l.allIssues
	}
	if l.cursor >= len(l.issues) && len(l.issues) > 0 {
		l.cursor = len(l.issues) - 1
	}
	l.tbl = l.buildTable()
	return l
}

// SetSort applies an in-memory sort without fetching new data.
func (l IssueList) SetSort(field string, asc bool) IssueList {
	l.sortField = field
	l.sortAsc = asc
	if field != "" {
		l.allIssues = sortIssues(l.allIssues, field, asc)
	}
	if l.filterQuery != "" {
		l.issues = applyFuzzyFilter(l.filterQuery, l.allIssues)
	} else {
		l.issues = l.allIssues
	}
	l.cursor = 0
	l.tbl = l.buildTable()
	return l
}

func (l IssueList) SortField() string { return l.sortField }
func (l IssueList) SortAsc() bool     { return l.sortAsc }

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

// HasMore returns true when there are more items on the server than loaded.
func (l IssueList) HasMore() bool {
	return l.total > len(l.allIssues)
}

// LoadedCount returns the number of issues fetched from the server (unfiltered).
func (l IssueList) LoadedCount() int { return len(l.allIssues) }

// IsFilterActive reports whether the filter input bar is visible.
func (l IssueList) IsFilterActive() bool { return l.filterActive }

// IsFilterApplied reports whether a filter query is currently narrowing results.
func (l IssueList) IsFilterApplied() bool { return l.filterQuery != "" }

// ActivateFilter opens the filter input bar.
func (l IssueList) ActivateFilter() IssueList {
	l.filterActive = true
	l.filterInput.Focus()
	l.filterInput.SetValue(l.filterQuery)
	l.filterInput.CursorEnd()
	l.tbl = l.buildTable()
	return l
}

// DeactivateFilter closes the filter bar while keeping the filter applied.
func (l IssueList) DeactivateFilter() IssueList {
	l.filterActive = false
	l.filterInput.Blur()
	l.tbl = l.buildTable()
	return l
}

// ClearFilter removes the filter and restores the full issue list.
func (l IssueList) ClearFilter() IssueList {
	l.filterActive = false
	l.filterQuery = ""
	l.filterInput.SetValue("")
	l.filterInput.Blur()
	l.issues = l.allIssues
	l.cursor = 0
	l.tbl = l.buildTable()
	return l
}

// UpdateFilter forwards a key/msg to the filter input and re-applies fuzzy filtering.
func (l IssueList) UpdateFilter(msg tea.Msg) (IssueList, tea.Cmd) {
	var cmd tea.Cmd
	l.filterInput, cmd = l.filterInput.Update(msg)
	l.filterQuery = l.filterInput.Value()
	if l.filterQuery != "" {
		l.issues = applyFuzzyFilter(l.filterQuery, l.allIssues)
	} else {
		l.issues = l.allIssues
	}
	l.cursor = 0
	l.tbl = l.buildTable()
	return l, cmd
}

func (l IssueList) SpinnerTick() (IssueList, tea.Cmd) {
	var cmd tea.Cmd
	l.spinner, cmd = l.spinner.Update(l.spinner.Tick())
	return l, cmd
}

func (l IssueList) Init() tea.Cmd {
	return l.spinner.Tick
}

// colWidth returns the effective pixel width for a configured column.
// Width==0 in config means "use the field default" (0 = flexible for summary).
func colWidth(c config.ListColumn) int {
	if c.Width > 0 {
		return c.Width
	}
	if w, ok := columnDefaultWidths[c.Field]; ok {
		return w
	}
	return 10
}

// colLabel returns the header label for a configured column.
func colLabel(c config.ListColumn) string {
	if c.Label != "" {
		return c.Label
	}
	if l, ok := columnDefaultLabels[c.Field]; ok {
		return l
	}
	return c.Field
}

// issueFieldValue extracts the value for a field ID from an Issue.
func issueFieldValue(iss model.Issue, field string) string {
	switch field {
	case "key":
		return iss.Key
	case "type":
		return iss.Type
	case "summary":
		return iss.Summary
	case "priority":
		return iss.Priority
	case "status":
		return iss.Status
	case "duedate":
		return iss.DueDate
	case "assignee":
		return iss.Assignee
	case "created":
		return iss.Created
	case "updated":
		return iss.Updated
	}
	return ""
}

// sortIssues returns a sorted copy of issues; original slice is not modified.
func sortIssues(issues []model.Issue, field string, asc bool) []model.Issue {
	if field == "" || len(issues) == 0 {
		return issues
	}
	sorted := make([]model.Issue, len(issues))
	copy(sorted, issues)
	sort.SliceStable(sorted, func(i, j int) bool {
		var less bool
		if field == "key" {
			less = issueKeyLess(sorted[i].Key, sorted[j].Key)
		} else {
			vi := strings.ToLower(issueFieldValue(sorted[i], field))
			vj := strings.ToLower(issueFieldValue(sorted[j], field))
			less = vi < vj
		}
		if asc {
			return less
		}
		return !less
	})
	return sorted
}

// issueKeyLess compares two Jira keys (e.g. PROJ-10 < PROJ-20) numerically.
func issueKeyLess(a, b string) bool {
	ai := strings.LastIndex(a, "-")
	bi := strings.LastIndex(b, "-")
	if ai < 0 || bi < 0 {
		return a < b
	}
	aPrefix, bPrefix := a[:ai], b[:bi]
	if aPrefix != bPrefix {
		return aPrefix < bPrefix
	}
	an, aerr := strconv.Atoi(a[ai+1:])
	bn, berr := strconv.Atoi(b[bi+1:])
	if aerr != nil || berr != nil {
		return a[ai+1:] < b[bi+1:]
	}
	return an < bn
}

func (l IssueList) buildTable() table.Model {
	if l.width == 0 {
		return table.Model{}
	}

	columns := l.columns
	if len(columns) == 0 {
		columns = []config.ListColumn{
			{Field: "key"}, {Field: "type"}, {Field: "summary"}, {Field: "status"},
		}
	}

	// Find the flexible column index (effective width == 0).
	flexIdx := -1
	for i, c := range columns {
		if colWidth(c) == 0 {
			flexIdx = i
			break
		}
	}
	// If none explicitly flexible, make the last column flexible.
	if flexIdx == -1 {
		flexIdx = len(columns) - 1
	}

	// Each column gains 2 chars from cell Padding(0,1); outer NormalBorder adds 2.
	borders := len(columns)*2 + 2
	fixedW := 0
	for i, c := range columns {
		if i != flexIdx {
			fixedW += colWidth(c)
		}
	}
	flexW := l.width - fixedW - borders
	if flexW < 10 {
		flexW = 10
	}

	cols := make([]table.Column, len(columns))
	for i, c := range columns {
		w := colWidth(c)
		if i == flexIdx {
			w = flexW
		}
		title := colLabel(c)
		if l.sortField == c.Field {
			if l.sortAsc {
				title += " ▲"
			} else {
				title += " ▼"
			}
		}
		cols[i] = table.Column{Title: title, Width: w}
	}

	rows := make([]table.Row, 0, len(l.issues))
	for _, iss := range l.issues {
		row := make(table.Row, len(columns))
		for i, c := range columns {
			v := issueFieldValue(iss, c.Field)
			maxW := cols[i].Width - 2
			if maxW > 0 && len(v) > maxW {
				v = v[:maxW-1] + "…"
			}
			row[i] = v
		}
		rows = append(rows, row)
	}

	tableHeight := l.height - 4
	if l.filterActive || l.filterQuery != "" {
		tableHeight--
	}
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
		if l.filterQuery != "" {
			sb.WriteString(statusIdleStyle.Padding(1, 2).Render(
				fmt.Sprintf("No matches for %q.", l.filterQuery)))
		} else {
			sb.WriteString(statusIdleStyle.Padding(1, 2).Render("No issues found."))
		}
		sb.WriteString(l.filterBarView())
		return sb.String()
	}

	sb.WriteString(tableBaseStyle.Render(l.tbl.View()))

	if l.loadingMore {
		sb.WriteString("\n" + loadMoreStyle.Render("  Loading more…"))
	} else if l.HasMore() {
		sb.WriteString("\n" + loadMoreStyle.Render(
			fmt.Sprintf("  %d of %d — press j at bottom to load more", len(l.allIssues), l.total)))
	}

	sb.WriteString(l.filterBarView())

	return sb.String()
}

func (l IssueList) filterBarView() string {
	if l.filterActive {
		hint := filterHintStyle.Render("  Enter to apply · Esc to clear")
		return "\n" + filterBarActiveStyle.Render("/") + " " + l.filterInput.View() + hint
	}
	if l.filterQuery != "" {
		count := fmt.Sprintf("(%d of %d)", len(l.issues), len(l.allIssues))
		hint := filterHintStyle.Render(fmt.Sprintf("  %s  / to re-edit · Esc to clear", count))
		return "\n" + filterBarAppliedStyle.Render("filter: "+l.filterQuery) + hint
	}
	return ""
}

// applyFuzzyFilter filters issues using fuzzy matching across all visible fields.
func applyFuzzyFilter(pattern string, issues []model.Issue) []model.Issue {
	if pattern == "" || len(issues) == 0 {
		return issues
	}
	targets := make([]string, len(issues))
	for i, iss := range issues {
		targets[i] = strings.Join(
			[]string{iss.Key, iss.Type, iss.Summary, iss.Priority, iss.Status, iss.DueDate}, " ")
	}
	matches := fuzzy.Find(pattern, targets)
	result := make([]model.Issue, 0, len(matches))
	for _, m := range matches {
		result = append(result, issues[m.Index])
	}
	return result
}
