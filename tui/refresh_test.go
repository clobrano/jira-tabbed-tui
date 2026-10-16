package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
)

// jiraStub serves total issues in pages of 100. Each request is counted;
// newTop issues are listed first (as if just created) and fail makes every
// request fail.
type jiraStub struct {
	total, newTop, calls int
	fail                 bool
}

func (j *jiraStub) pages(jql, cursor string) (backend.SearchPage, error) {
	j.calls++
	if j.fail {
		return backend.SearchPage{}, errors.New("boom")
	}
	var keys []string
	for i := 0; i < j.newTop; i++ {
		keys = append(keys, fmt.Sprintf("NEW-%d", i+1))
	}
	for i := 0; i < j.total; i++ {
		keys = append(keys, fmt.Sprintf("P-%d", i+1))
	}
	start, _ := strconv.Atoi(cursor)
	end := min(start+backend.SearchPageSize, len(keys))
	page := backend.SearchPage{Total: len(keys)}
	for _, k := range keys[start:end] {
		page.Issues = append(page.Issues, model.Issue{Key: k, Status: "To Do"})
	}
	if end < len(keys) {
		page.Next = strconv.Itoa(end)
	}
	return page, nil
}

// refreshTestApp has two loaded tabs (1 and 2, the first active) on a clock
// the test moves with advance.
func refreshTestApp(t *testing.T, stub *jiraStub) (a App, advance func(time.Duration)) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfgYAML := "tabs:\n  - name: Mine\n    jql: assignee = currentUser()\n  - name: Team\n    jql: project = P\n"
	if err := os.WriteFile(path, []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := New(cfg, path, noopRunner{}).Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	a = m.(App)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	a.cache.SetClock(a.now)
	a.pages = stub.pages

	a, cmd := a.loadTab(1, true)
	a = runLoad(t, a, cmd)
	m, cmd = a.switchToTab(2)
	a = runLoad(t, m.(App), cmd)
	m, cmd = a.switchToTab(1)
	a = runLoad(t, m.(App), cmd)
	return a, func(d time.Duration) { now = now.Add(d) }
}

func TestTabSwitchUsesFreshList(t *testing.T) {
	stub := &jiraStub{total: 250}
	a, advance := refreshTestApp(t, stub)
	if stub.calls != 6 { // 3 pages for each tab
		t.Fatalf("initial loads made %d requests, want 6", stub.calls)
	}

	advance(2 * time.Minute)
	m, cmd := a.switchToTab(2)
	a = runLoad(t, m.(App), cmd)
	m, cmd = a.switchToTab(1)
	a = runLoad(t, m.(App), cmd)
	if stub.calls != 6 {
		t.Errorf("switching between fresh tabs made %d requests", stub.calls-6)
	}
	if n := len(a.tabs[1].list.allIssues); n != 250 {
		t.Errorf("tab shows %d issues, want 250", n)
	}
	got := stripANSI(a.titleBar())
	if !strings.HasSuffix(got, "updated 2m ago · auto-refresh in 28m · r refresh ") {
		t.Errorf("title bar: %q", got)
	}
}

func TestStaleTabRefreshesInBackground(t *testing.T) {
	stub := &jiraStub{total: 250}
	a, advance := refreshTestApp(t, stub)
	a = send(t, a, keyMsg("j"), keyMsg("j")) // cursor on P-3

	advance(31 * time.Minute)
	stub.newTop = 2 // two issues created meanwhile, listed first
	m, cmd := a.switchToTab(2)
	a = m.(App)
	if cmd == nil || !a.tabs[2].refreshing {
		t.Fatal("switching to a stale tab did not refresh it")
	}
	a = runLoad(t, a, cmd)
	m, cmd = a.switchToTab(1)
	a = m.(App)

	// The first page arrives: the old rows stay, the title bar shows progress.
	m, next := a.Update(cmd())
	a = m.(App)
	l := a.tabs[1].list
	if len(l.allIssues) != 250 || l.loading {
		t.Fatalf("mid-refresh the list shows %d issues (loading %v), want the old 250", len(l.allIssues), l.loading)
	}
	if got := stripANSI(a.titleBar()); !strings.HasSuffix(got, "refreshing 100 of 252… ") {
		t.Errorf("mid-refresh title bar: %q", got)
	}

	// Once complete the new list swaps in, cursor still on P-3.
	a = runLoad(t, a, next)
	l = a.tabs[1].list
	if len(l.allIssues) != 252 || a.tabs[1].refreshing {
		t.Fatalf("after refresh: %d issues, refreshing %v", len(l.allIssues), a.tabs[1].refreshing)
	}
	if iss, _ := l.SelectedIssue(); iss.Key != "P-3" {
		t.Errorf("cursor moved to %s, want P-3", iss.Key)
	}
	if got := stripANSI(a.titleBar()); !strings.Contains(got, "updated 0s ago") {
		t.Errorf("title bar after refresh: %q", got)
	}
}

func TestForceRefreshKeepsRows(t *testing.T) {
	stub := &jiraStub{total: 250}
	a, _ := refreshTestApp(t, stub)
	calls := stub.calls

	m, cmd := a.Update(keyMsg("r"))
	a = m.(App)
	if !a.tabs[1].refreshing || a.tabs[1].list.loading || len(a.tabs[1].list.allIssues) != 250 {
		t.Fatal("r blanked the list instead of refreshing it in the background")
	}
	a = runLoad(t, a, cmd)
	if stub.calls-calls != 3 {
		t.Errorf("r made %d requests, want 3 (the whole list)", stub.calls-calls)
	}
}

func TestFailedRefreshKeepsRows(t *testing.T) {
	stub := &jiraStub{total: 50}
	a, advance := refreshTestApp(t, stub)
	advance(time.Minute)
	stub.fail = true

	m, cmd := a.Update(keyMsg("r"))
	a = runLoad(t, m.(App), cmd)
	if n := len(a.tabs[1].list.allIssues); n != 50 || !a.tabs[1].refreshFailed {
		t.Fatalf("after a failed refresh: %d issues, refreshFailed %v", n, a.tabs[1].refreshFailed)
	}
	if got := stripANSI(a.titleBar()); !strings.HasSuffix(got, "refresh failed · data from 1m ago · r retry ") {
		t.Errorf("title bar: %q", got)
	}

	// The failed attempt is not retried on every tick, only after the TTL.
	calls := stub.calls
	advance(time.Minute)
	m, _ = a.Update(autoRefreshMsg{})
	if a = m.(App); a.tabs[1].refreshing || stub.calls != calls {
		t.Errorf("auto-refresh retried a minute after failing")
	}
	advance(30 * time.Minute)
	m, _ = a.Update(autoRefreshMsg{})
	if a = m.(App); !a.tabs[1].refreshing {
		t.Errorf("auto-refresh did not retry after the TTL")
	}
}

func TestAutoRefresh(t *testing.T) {
	stub := &jiraStub{total: 50}
	a, advance := refreshTestApp(t, stub)
	calls := stub.calls

	advance(29 * time.Minute)
	m, _ := a.Update(autoRefreshMsg{})
	a = m.(App)
	if a.tabs[1].refreshing || stub.calls != calls {
		t.Fatal("refreshed a fresh tab")
	}
	if got := stripANSI(a.titleBar()); !strings.Contains(got, "auto-refresh in 1m") {
		t.Errorf("title bar: %q", got)
	}

	advance(time.Minute)
	m, _ = a.Update(autoRefreshMsg{})
	a = m.(App)
	if !a.tabs[1].refreshing || a.tabs[2].refreshing {
		t.Fatal("the stale visible tab (and only it) should refresh")
	}
}
