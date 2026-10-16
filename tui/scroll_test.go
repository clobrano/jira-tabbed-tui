package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/model"
)

func longListApp(t *testing.T, n, total int) App {
	t.Helper()
	a := newTestApp(t)
	a = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 30})
	issues := make([]model.Issue, n)
	for i := range issues {
		issues[i] = model.Issue{Key: fmt.Sprintf("P-%d", i+1), Summary: "s", Status: "To Do"}
	}
	return send(t, a, backend.ListFetchedMsg{TabIdx: 1, Gen: a.tabs[1].loadGen, Issues: issues, Total: total})
}

func selectedKey(a App) string {
	iss, _ := a.tabs[a.activeTab].list.SelectedIssue()
	return iss.Key
}

// keyOnScreen reports whether an issue key is rendered as a whole word.
func keyOnScreen(screen, key string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `\b`).MatchString(screen)
}

func TestCursorStaysOnScreenInLongList(t *testing.T) {
	a := longListApp(t, 50, 50)
	for i := 0; i < 49; i++ {
		a = send(t, a, keyMsg("j"))
		screen := stripANSI(a.View())
		if !keyOnScreen(screen, selectedKey(a)) {
			t.Fatalf("after %d× j, selected %s is not on screen:\n%s", i+1, selectedKey(a), screen)
		}
		if h := lipgloss.Height(screen); h > 30 {
			t.Fatalf("screen is %d rows, terminal has 30", h)
		}
	}
	// And all the way back up.
	for i := 0; i < 49; i++ {
		a = send(t, a, keyMsg("k"))
		if screen := stripANSI(a.View()); !keyOnScreen(screen, selectedKey(a)) {
			t.Fatalf("after %d× k, selected %s is not on screen", i+1, selectedKey(a))
		}
	}
}

func TestPositionLineCountsRowsOutOfView(t *testing.T) {
	a := longListApp(t, 50, 120)
	l := a.tabs[1].list
	vis := l.visibleRows()

	if got := l.positionLine(); got != fmt.Sprintf("row 1 of 50 · ↓ %d below", 50-vis) {
		t.Errorf("top: %q", got)
	}

	// Scrolling keeps scrollMargin rows of context below the cursor.
	for i := 0; i < 30; i++ {
		a = send(t, a, keyMsg("j"))
	}
	l = a.tabs[1].list
	above := 30 + scrollMargin - vis + 1
	want := fmt.Sprintf("row 31 of 50 · ↑ %d above · ↓ %d below", above, 50-above-vis)
	if got := l.positionLine(); !strings.HasPrefix(got, want) {
		t.Errorf("middle: %q, want prefix %q", got, want)
	}

	// Moving back up inside the window doesn't scroll.
	a = send(t, a, keyMsg("k"))
	if got := a.tabs[1].list.offset; got != above {
		t.Errorf("k inside the window scrolled: offset %d, want %d", got, above)
	}

	a = send(t, a, keyMsg("G"))
	if got := a.tabs[1].list.positionLine(); got != fmt.Sprintf("row 50 of 50 · ↑ %d above", 50-vis) {
		t.Errorf("bottom: %q", got)
	}
	if !strings.Contains(stripANSI(a.View()), "row 50 of 50") {
		t.Error("position line not rendered")
	}
}
