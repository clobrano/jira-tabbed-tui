package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
)

func TestHLSwitchTabsInListView(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfgYAML := "tabs:\n  - name: Mine\n    jql: a = b\n  - name: Team\n    jql: c = d\n"
	if err := os.WriteFile(path, []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := New(cfg, path, noopRunner{}).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a := m.(App) // tabs: 0 Search, 1 Mine, 2 Team

	a = send(t, a, keyMsg("l"))
	if a.activeTab != 2 {
		t.Errorf("l: active tab = %d, want 2", a.activeTab)
	}
	a = send(t, a, keyMsg("h"))
	if a.activeTab != 1 {
		t.Errorf("h: active tab = %d, want 1", a.activeTab)
	}

	// On the Search tab the JQL field has focus: h/l are typed, like the
	// arrows move its cursor; Tab still leaves.
	a = send(t, a, keyMsg("h"), keyMsg("l"))
	if a.activeTab != 0 || a.tabs[0].search.input.Value() != "l" {
		t.Errorf("search tab: active=%d query=%q, want 0 and \"l\"", a.activeTab, a.tabs[0].search.input.Value())
	}
}

func TestHLSwitchBodyTabsInDetailView(t *testing.T) {
	a := newTestApp(t)
	a.view = viewDetail
	a = send(t, a, fetched("P-1", "P-2"))
	for _, step := range []struct {
		key  string
		want detailBodyTab
	}{{"l", bodyTabComments}, {"l", bodyTabLinks}, {"h", bodyTabComments}} {
		a = send(t, a, keyMsg(step.key))
		if got := a.detail.ViewState().bodyTab; got != step.want {
			t.Errorf("after %s: body tab = %d, want %d", step.key, got, step.want)
		}
	}
	if a.overlay != overlayNone {
		t.Errorf("l opened an overlay (%d); labels moved to L", a.overlay)
	}
}

func TestShiftLAddsLabels(t *testing.T) {
	a := newTestApp(t)
	a.view = viewDetail
	a = send(t, a, fetched("P-1"), keyMsg("L"))
	if a.overlay != overlayLabels {
		t.Errorf("L: overlay = %d, want labels", a.overlay)
	}
}

func TestActionBoundToLKeepsIt(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Keybindings.AddLabels = "l" // e.g. an older config that set it explicitly
	a.view = viewDetail
	a = send(t, a, fetched("P-1"), keyMsg("l"))
	if a.overlay != overlayLabels {
		t.Errorf("l bound to add_labels: overlay = %d, want labels", a.overlay)
	}
	if got := a.detail.ViewState().bodyTab; got != bodyTabDescription {
		t.Errorf("l bound to an action still switched body tab to %d", got)
	}
}

func TestVimTopBottom(t *testing.T) {
	a := newTestApp(t)
	issues := []model.Issue{{Key: "P-1"}, {Key: "P-2"}, {Key: "P-3"}}
	a = send(t, a, backend.ListFetchedMsg{TabIdx: 1, Issues: issues, Total: 3})
	cursor := func() string { iss, _ := a.tabs[1].list.SelectedIssue(); return iss.Key }

	a = send(t, a, keyMsg("G"))
	if cursor() != "P-3" {
		t.Errorf("G: cursor on %s, want P-3", cursor())
	}
	// A single g waits; a different key cancels it.
	a = send(t, a, keyMsg("g"), keyMsg("k"), keyMsg("g"))
	if cursor() != "P-2" {
		t.Errorf("g k g: cursor on %s, want P-2 (no jump)", cursor())
	}
	a = send(t, a, keyMsg("g"))
	if cursor() != "P-1" {
		t.Errorf("gg: cursor on %s, want P-1", cursor())
	}

	// Detail view, Links tab: first/last link.
	a.view = viewDetail
	a = send(t, a, fetched("P-1", "L-1", "L-2", "L-3"), keyMsg("l"), keyMsg("l"), keyMsg("G"))
	if l, _ := a.detail.SelectedLink(); l.Key != "L-3" {
		t.Errorf("detail G: link %s, want L-3", l.Key)
	}
	a = send(t, a, keyMsg("g"), keyMsg("g"))
	if l, _ := a.detail.SelectedLink(); l.Key != "L-1" {
		t.Errorf("detail gg: link %s, want L-1", l.Key)
	}
}
