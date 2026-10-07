package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
)

type noopRunner struct{}

func (noopRunner) Run(...string) ([]byte, error) { return nil, nil }

func newTestApp(t *testing.T) App {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tabs:\n  - name: Mine\n    jql: assignee = currentUser()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := New(cfg, path, noopRunner{}).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m.(App)
}

func send(t *testing.T, a App, msgs ...tea.Msg) App {
	t.Helper()
	for _, msg := range msgs {
		m, _ := a.Update(msg)
		a = m.(App)
	}
	return a
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+i":
		return tea.KeyMsg{Type: tea.KeyCtrlI}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func fetched(key string, links ...string) backend.DetailFetchedMsg {
	d := model.IssueDetail{Issue: model.Issue{Key: key}}
	for _, l := range links {
		d.Links = append(d.Links, model.IssueLink{Type: "relates to", Key: l})
	}
	return backend.DetailFetchedMsg{Issue: d}
}

// Going back from a linked issue returns to the parent's Links tab with the
// cursor on the link that was opened, and forward restores the child's view.
func TestDetailHistoryRestoresLinksView(t *testing.T) {
	a := newTestApp(t)
	a.view = viewDetail
	a = a.pushHistory("P-1")
	a = send(t, a, fetched("P-1", "C-1", "C-2"), keyMsg("right"), keyMsg("right"), keyMsg("j"))
	if l, ok := a.detail.SelectedLink(); !ok || l.Key != "C-2" {
		t.Fatalf("selected link = %+v, %v; want C-2 on Links tab", l, ok)
	}

	// Open C-2, then switch it to its Comments tab.
	a = send(t, a, keyMsg("enter"), fetched("C-2"), keyMsg("right"))
	if a.detail.IssueKey() != "C-2" || a.detail.ViewState().bodyTab != bodyTabComments {
		t.Fatalf("want C-2 on Comments, got %s %+v", a.detail.IssueKey(), a.detail.ViewState())
	}

	// Back: parent on Links with C-2 selected again.
	a = send(t, a, keyMsg("backspace"), fetched("P-1", "C-1", "C-2"))
	if l, ok := a.detail.SelectedLink(); !ok || l.Key != "C-2" {
		t.Fatalf("after back: selected link = %+v, %v; want C-2 on Links tab", l, ok)
	}

	// Forward: child back on Comments.
	a = send(t, a, keyMsg("ctrl+i"), fetched("C-2"))
	if a.detail.IssueKey() != "C-2" || a.detail.ViewState().bodyTab != bodyTabComments {
		t.Fatalf("after forward: want C-2 on Comments, got %s %+v", a.detail.IssueKey(), a.detail.ViewState())
	}
}

func TestChildrenAlreadyListedAsSubtasksAreNotDuplicated(t *testing.T) {
	a := newTestApp(t)
	a.view = viewDetail
	a = send(t, a, fetched("P-1", "S-1"), backend.ChildrenFetchedMsg{
		ParentKey: "P-1",
		Children:  []model.Issue{{Key: "S-1"}, {Key: "E-1"}},
	})
	var keys []string
	for _, l := range a.detail.Issue().Links {
		keys = append(keys, l.Key)
	}
	if len(keys) != 2 || keys[0] != "E-1" || keys[1] != "S-1" {
		t.Fatalf("links = %v, want [E-1 S-1] (deduplicated, sorted by key)", keys)
	}
}

func TestRefetchKeepsSuccessMessageButClearsErrors(t *testing.T) {
	a := newTestApp(t)
	a.statusLine = a.statusLine.SetMessage("transition succeeded", false)
	a = send(t, a, fetched("P-1"), backend.ListFetchedMsg{TabIdx: 1})
	if a.statusLine.message != "transition succeeded" {
		t.Errorf("success message cleared by refetch: %q", a.statusLine.message)
	}
	a.statusLine = a.statusLine.SetMessage("boom", true)
	a = send(t, a, fetched("P-1"))
	if a.statusLine.message != "" {
		t.Errorf("stale error kept after refetch: %q", a.statusLine.message)
	}
}

// fetchingRunner serves `issue view KEY --raw` with a status it can change.
type fetchingRunner struct {
	status string
	err    error
}

func (r *fetchingRunner) Run(args ...string) ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	if len(args) >= 3 && args[0] == "issue" && args[1] == "view" {
		return []byte(`{"key":"` + args[2] + `","fields":{"summary":"s","status":{"name":"` + r.status + `"},
			"issuelinks":[{"type":{"inward":"x","outward":"relates to"},"outwardIssue":{"key":"P-2","fields":{"summary":"l"}}}]}}`), nil
	}
	return []byte(`{"total":0,"issues":[]}`), nil
}

func TestRefreshInDetailViewReloadsIssueInPlace(t *testing.T) {
	r := &fetchingRunner{status: "To Do"}
	a := newTestApp(t)
	a.runner = r
	a.view = viewDetail
	a = a.pushHistory("P-1")

	// Load P-1 through the cache, then move to its Links tab.
	a = send(t, a, a.cache.FetchDetailCmd(r, "P-1")(), keyMsg("right"), keyMsg("right"))
	if a.detail.Issue().Status != "To Do" {
		t.Fatalf("status = %q", a.detail.Issue().Status)
	}

	// The issue changes in Jira; r reloads it, bypassing the cache.
	r.status = "Done"
	m, cmd := a.Update(keyMsg("r"))
	a = m.(App)
	if cmd == nil {
		t.Fatal("r in the detail view did nothing")
	}
	a = send(t, a, cmd())
	if a.detail.Issue().Status != "Done" {
		t.Errorf("after refresh status = %q, want Done", a.detail.Issue().Status)
	}
	if l, ok := a.detail.SelectedLink(); !ok || l.Key != "P-2" {
		t.Errorf("refresh lost the Links tab selection: %+v %v", l, ok)
	}
	if a.statusLine.message != "refreshed P-1" || a.statusLine.isError {
		t.Errorf("status line = %q (error=%v)", a.statusLine.message, a.statusLine.isError)
	}

	// A failed refresh keeps the issue on screen and reports the error.
	r.err = errors.New("network down")
	m, cmd = a.Update(keyMsg("r"))
	a = send(t, m.(App), cmd())
	if a.detail.IssueKey() != "P-1" || a.detail.Issue().Status != "Done" {
		t.Errorf("failed refresh replaced the issue: %q %q", a.detail.IssueKey(), a.detail.Issue().Status)
	}
	if !a.statusLine.isError || !strings.Contains(a.statusLine.message, "refresh failed") {
		t.Errorf("status line = %q (error=%v)", a.statusLine.message, a.statusLine.isError)
	}
}
