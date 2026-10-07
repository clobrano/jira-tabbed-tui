package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/model"
)

func titleTestApp(t *testing.T, width int) App {
	t.Helper()
	a := newTestApp(t).WithUser("dana@acme.com\n")
	a.server = "acme.atlassian.net"
	a.width = width
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	issues := []model.Issue{
		{Key: "P-1", Status: "In Progress"}, {Key: "P-2", Status: "In Progress"},
		{Key: "P-3", Status: "To Do"}, {Key: "P-4", Status: "In Review"},
	}
	return send(t, a, backend.ListFetchedMsg{TabIdx: 1, Issues: issues, Total: 4, At: now.Add(-90 * time.Second)})
}

func TestTitleBarWide(t *testing.T) {
	a := titleTestApp(t, 200)
	got := stripANSI(a.titleBar())
	want := " JIRA-TABBED-TUI  Jira, one tab per query · acme.atlassian.net · dana@acme.com · 4 issues 2 In Progress 1 In Review 1 To Do"
	if !strings.HasPrefix(got, want) {
		t.Errorf("left side:\n got %q\nwant %q", got, want)
	}
	if !strings.HasSuffix(got, "updated 1m ago ") {
		t.Errorf("right side: %q", got)
	}
	if w := lipgloss.Width(got); w != 200 {
		t.Errorf("width = %d, want 200", w)
	}
}

func TestTitleBarGivesWayOnNarrowScreens(t *testing.T) {
	a := titleTestApp(t, 80)
	got := stripANSI(a.titleBar())
	if lipgloss.Width(got) > 80 {
		t.Fatalf("80 columns: title bar is %d wide: %q", lipgloss.Width(got), got)
	}
	// The tagline and user give way first; name, count and freshness stay.
	for _, keep := range []string{"JIRA-TABBED-TUI", "4 issues", "updated 1m ago"} {
		if !strings.Contains(got, keep) {
			t.Errorf("80 columns lost %q: %q", keep, got)
		}
	}
	for _, gone := range []string{"one tab per query", "dana@acme.com"} {
		if strings.Contains(got, gone) {
			t.Errorf("80 columns kept low-priority %q: %q", gone, got)
		}
	}
	// Tiny: still one line, never wider than the screen.
	a.width = 20
	if got := stripANSI(a.titleBar()); lipgloss.Width(got) > 20 || strings.Contains(got, "\n") {
		t.Errorf("20 columns: %q", got)
	}
}

func TestTitleBarRefreshFailed(t *testing.T) {
	a := titleTestApp(t, 200)
	a = send(t, a, backend.ListFetchedMsg{
		TabIdx: 1, Issues: a.tabs[1].list.allIssues, Total: 4,
		Err: errors.New("boom"), Stale: true, At: a.now().Add(-6 * time.Minute),
	})
	if got := stripANSI(a.titleBar()); !strings.HasSuffix(got, "refresh failed · data from 6m ago ") {
		t.Errorf("stale title bar: %q", got)
	}
}

func TestStatusColor(t *testing.T) {
	for status, want := range map[string]lipgloss.Color{
		"To Do": statusTodoColor, "Backlog": statusTodoColor, "In Progress": statusProgressColor,
		"In Review": statusReviewColor, "QA": statusReviewColor, "Done": statusDoneColor, "Closed": statusDoneColor,
	} {
		if got := statusColor(status); got != want {
			t.Errorf("statusColor(%q) = %v, want %v", status, got, want)
		}
	}
}
