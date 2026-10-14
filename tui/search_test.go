package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/clobrano/jira-tabbed-tui/config"
)

func typeText(t *testing.T, a App, s string) App {
	t.Helper()
	for _, r := range s {
		a = send(t, a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return a
}

func TestSearchTabFocusOnlyOnQ(t *testing.T) {
	a := newTestApp(t) // tabs: 0 Search, 1 Mine; starts on Mine
	a = send(t, a, keyMsg("h"))
	s := a.tabs[0].search
	if a.activeTab != 0 || s.IsFocused() {
		t.Fatalf("arriving on Search: active=%d focused=%v, want unfocused", a.activeTab, s.IsFocused())
	}
	if screen := stripANSI(a.View()); !strings.Contains(screen, "Press e to type a JQL query") {
		t.Errorf("empty Search tab doesn't say how to start:\n%s", screen)
	}
	if got := hintsPlain(a.listHints()); !strings.HasPrefix(got, "e type a JQL query") {
		t.Errorf("empty Search footer = %q", got)
	}

	// e focuses; inside the field e (and Q) are typed, not "edit JQL".
	a = send(t, a, keyMsg("e"))
	a = typeText(t, a, "aQeb")
	if a.overlay != overlayNone {
		t.Fatalf("typing Q in the field opened overlay %d", a.overlay)
	}
	if got := a.tabs[0].search.input.Value(); got != "aQeb" {
		t.Errorf("field = %q, want aQeb", got)
	}
	if got := hintsPlain(a.listHints()); !strings.Contains(got, "esc leave field") {
		t.Errorf("focused footer = %q", got)
	}

	// Esc leaves the field and keeps the text; tab keys work again.
	a = send(t, a, tea.KeyMsg{Type: tea.KeyEsc})
	if a.tabs[0].search.IsFocused() || a.tabs[0].search.input.Value() != "aQeb" {
		t.Errorf("after Esc: focused=%v text=%q", a.tabs[0].search.IsFocused(), a.tabs[0].search.input.Value())
	}
	a = send(t, a, keyMsg("l"))
	if a.activeTab != 1 {
		t.Errorf("l after Esc: active tab %d, want 1", a.activeTab)
	}

	// Back on Search, e resumes editing the kept text and Enter runs it.
	a = send(t, a, keyMsg("h"), keyMsg("e"))
	m, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = m.(App)
	if cmd == nil {
		t.Fatal("Enter didn't run the query")
	}
	if msg, ok := cmd().(SearchRunMsg); !ok || msg.Query != "aQeb" {
		t.Errorf("Enter ran %#v, want SearchRunMsg{aQeb}", cmd())
	}
	if a.tabs[0].search.IsFocused() {
		t.Error("field still focused after running the query")
	}
}

func TestSearchFocusedAtStartupWhenOnlyTab(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tabs: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a := New(cfg, path, noopRunner{})
	if a.activeTab != 0 || !a.tabs[0].search.IsFocused() {
		t.Errorf("only Search tab: active=%d focused=%v, want focused", a.activeTab, a.tabs[0].search.IsFocused())
	}
}
