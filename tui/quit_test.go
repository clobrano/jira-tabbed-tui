package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func quits(t *testing.T, a App, msg tea.KeyMsg) bool {
	t.Helper()
	_, cmd := a.Update(msg)
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

var ctrlC = tea.KeyMsg{Type: tea.KeyCtrlC}

func TestQuitFromListAndTaskView(t *testing.T) {
	a := newTestApp(t)
	if !quits(t, a, keyMsg("q")) || !quits(t, a, ctrlC) {
		t.Error("q / ctrl+c don't quit from the list view")
	}

	a.view = viewDetail
	a = send(t, a, fetched("P-1"))
	if !quits(t, a, keyMsg("q")) {
		t.Error("q doesn't quit from the task view")
	}
	if !quits(t, a, ctrlC) {
		t.Error("ctrl+c doesn't quit from the task view")
	}
}

func TestQIsTypedInTextFields(t *testing.T) {
	a := newTestApp(t)

	// Search tab's JQL bar: q is a letter, not quit.
	a = send(t, a, keyMsg("h"), keyMsg("e"))
	m, cmd := a.Update(keyMsg("q"))
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("q in the Search JQL bar quit the app")
		}
	}
	a = m.(App)
	if got := a.tabs[0].search.input.Value(); got != "q" {
		t.Errorf("JQL bar = %q, want q", got)
	}
	if !quits(t, a, ctrlC) {
		t.Error("ctrl+c doesn't quit while typing a query")
	}

	// Fuzzy filter on a normal tab.
	a = send(t, a, tea.KeyMsg{Type: tea.KeyEsc}, keyMsg("l"), keyMsg("/"))
	if quits(t, a, keyMsg("q")) {
		t.Error("q in the fuzzy filter quit the app")
	}
}

func TestQInDialogsKeepsItsMeaning(t *testing.T) {
	a := newTestApp(t)
	a.view = viewDetail
	a = send(t, a, fetched("P-1"), keyMsg("c")) // add-comment dialog (text)
	if a.overlay == overlayComment && quits(t, a, keyMsg("q")) {
		t.Error("q in the comment dialog quit the app")
	}

	a = newTestApp(t)
	a = send(t, a, keyMsg("?"))
	if quits(t, a, keyMsg("q")) {
		t.Error("q on the help overlay quit instead of closing it")
	}
	if a = send(t, a, keyMsg("q")); a.overlay != overlayNone {
		t.Error("q didn't close the help overlay")
	}
	if !quits(t, send(t, a, keyMsg("?")), ctrlC) {
		t.Error("ctrl+c doesn't quit from the help overlay")
	}
}
