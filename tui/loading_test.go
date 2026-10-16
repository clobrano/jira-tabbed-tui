package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/model"
)

// fakePages serves total issues in pages of 100; the cursor is the next
// start index. calls counts the requests.
func fakePages(total int, calls *int) backend.PageFunc {
	return func(jql, cursor string) (backend.SearchPage, error) {
		*calls++
		start, _ := strconv.Atoi(cursor)
		n := min(backend.SearchPageSize, total-start)
		page := backend.SearchPage{Total: total}
		for i := 0; i < n; i++ {
			page.Issues = append(page.Issues, model.Issue{Key: fmt.Sprintf("P-%d", start+i+1), Status: "To Do"})
		}
		if start+n < total {
			page.Next = strconv.Itoa(start + n)
		}
		return page, nil
	}
}

// runLoad feeds cmd's message to the app and keeps following the commands it
// returns (the background page loads) until there are none.
func runLoad(t *testing.T, a App, cmd tea.Cmd) App {
	t.Helper()
	for i := 0; cmd != nil; i++ {
		if i > 100 {
			t.Fatal("load never finished")
		}
		msg := cmd()
		if msg == nil {
			break
		}
		var m tea.Model
		m, cmd = a.Update(msg)
		a = m.(App)
	}
	return a
}

func TestListLoadsAllPagesInBackground(t *testing.T) {
	calls := 0
	a := newTestApp(t)
	a.pages = fakePages(250, &calls)

	m, cmd := a.Update(keyMsg("r")) // refresh the active tab
	a = m.(App)

	// The first page shows straight away, with the rest still loading.
	first := cmd()
	m, next := a.Update(first)
	a = m.(App)
	l := a.tabs[1].list
	if len(l.allIssues) != 100 || next == nil || !strings.Contains(l.positionLine(), "loading 100 of 250…") {
		t.Fatalf("after page 1: %d issues, next cmd %v, %q", len(l.allIssues), next != nil, l.positionLine())
	}

	a = runLoad(t, a, next)
	l = a.tabs[1].list
	if len(l.allIssues) != 250 || calls != 3 {
		t.Fatalf("loaded %d issues in %d calls, want 250 in 3", len(l.allIssues), calls)
	}
	if got := l.positionLine(); strings.Contains(got, "loading") {
		t.Errorf("still loading after the last page: %q", got)
	}
	if got := stripANSI(a.titleBar()); !strings.Contains(got, "250 issues") {
		t.Errorf("title bar = %q", got)
	}
}

func TestListLoadingStopsAtCap(t *testing.T) {
	calls := 0
	a := newTestApp(t)
	a.cfg.List.MaxIssues = 200
	a.pages = fakePages(1000, &calls)

	_, cmd := a.Update(keyMsg("r"))
	a = runLoad(t, a, cmd)
	l := a.tabs[1].list
	if len(l.allIssues) != 200 || calls != 2 {
		t.Fatalf("loaded %d issues in %d calls, want 200 in 2", len(l.allIssues), calls)
	}
	if got := l.positionLine(); !strings.Contains(got, "showing first 200 of 1000 — narrow the JQL") {
		t.Errorf("capped position line = %q", got)
	}
}

func TestSupersededLoadIsIgnored(t *testing.T) {
	calls := 0
	a := newTestApp(t)
	a.pages = fakePages(250, &calls)

	// Start a load and take its first page...
	m, cmd := a.Update(keyMsg("r"))
	a = m.(App)
	oldFirst := cmd()

	// ...but refresh again before it is applied.
	m, cmd = a.Update(keyMsg("r"))
	a = m.(App)
	m, stale := a.Update(oldFirst)
	a = m.(App)
	if stale != nil || len(a.tabs[1].list.allIssues) != 0 {
		t.Fatalf("superseded page was applied (%d issues, next cmd %v)", len(a.tabs[1].list.allIssues), stale != nil)
	}

	a = runLoad(t, a, cmd)
	if n := len(a.tabs[1].list.allIssues); n != 250 {
		t.Errorf("current load ended with %d issues, want 250", n)
	}
}
