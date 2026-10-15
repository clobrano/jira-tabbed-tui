package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestListFooterHints(t *testing.T) {
	a := newTestApp(t)
	a.activeTab = 1
	hints := a.listHints()

	// Wide: movement, then enter, then filter, sort and edit JQL. Issue
	// actions, refresh and quit live under "? all keys".
	want := "j/k move · h/l switch tab · gg/G top/bottom · enter open · / filter · s sort · e edit JQL · ? all keys"
	if got := hintsPlain(fitHints(hints, 300)); got != want {
		t.Errorf("wide footer:\n got %q\nwant %q", got, want)
	}

	// 80 columns: top/bottom and sort give way; edit JQL stays, nothing is
	// cut in half, and "? all keys" stays.
	narrow := hintsPlain(fitHints(hints, 78))
	if want := "j/k move · h/l switch tab · enter open · / filter · e edit JQL · ? all keys"; narrow != want {
		t.Errorf("80-column footer:\n got %q\nwant %q", narrow, want)
	}

	// Very narrow: only "? all keys" is left.
	if got := hintsPlain(fitHints(hints, 12)); got != "? all keys" {
		t.Errorf("tiny footer = %q", got)
	}

	// A sorted list says how, in the sort hint.
	a.tabs[1].list = a.tabs[1].list.SetSort("priority", false)
	if got := hintsPlain(fitHints(a.listHints(), 300)); !strings.Contains(got, "s sort (priority ▼)") {
		t.Errorf("sorted footer = %q", got)
	}
}

func TestDetailFooter(t *testing.T) {
	a := newTestApp(t)
	a.view = viewDetail
	a = a.pushHistory("P-1")
	a = send(t, a, fetched("P-1", "P-2"))

	// Description tab, no history yet.
	if got, want := hintsPlain(fitHints(a.detailHints(), 300)),
		"j/k scroll · h/l switch section · gg/G top/bottom · esc list · ? all keys"; got != want {
		t.Errorf("description footer:\n got %q\nwant %q", got, want)
	}

	// Links tab: j/k moves between links and enter names the target.
	a = send(t, a, keyMsg("l"), keyMsg("l"))
	if got, want := hintsPlain(fitHints(a.detailHints(), 300)),
		"j/k move · h/l switch section · gg/G top/bottom · enter open P-2 · esc list · ? all keys"; got != want {
		t.Errorf("links footer:\n got %q\nwant %q", got, want)
	}

	// After following the link, back is offered; forward once we go back.
	a = send(t, a, keyMsg("enter"), fetched("P-2"))
	if got := hintsPlain(fitHints(a.detailHints(), 300)); !strings.Contains(got, "⌫/ctrl+o back") || strings.Contains(got, "forward") {
		t.Errorf("footer after following a link = %q", got)
	}
	a = send(t, a, keyMsg("backspace"), fetched("P-1", "P-2"))
	if got := hintsPlain(fitHints(a.detailHints(), 300)); !strings.Contains(got, "ctrl+i forward") {
		t.Errorf("footer after going back = %q", got)
	}

	// Issue actions are under "? all keys", not in the footer.
	for _, gone := range []string{"change", "assign", "comment", "labels", "copy", "refresh", "fields"} {
		if got := hintsPlain(fitHints(a.detailHints(), 300)); strings.Contains(got, gone) {
			t.Errorf("footer still lists %q: %q", gone, got)
		}
	}

	// 80 columns: everything but top/bottom fits.
	if got := hintsPlain(fitHints(a.detailHints(), 78)); strings.Contains(got, "gg/G") || !strings.Contains(got, "enter open P-2") {
		t.Errorf("80-column footer = %q", got)
	}
}

func TestStatusLineHintRowFitsOneLine(t *testing.T) {
	a := newTestApp(t)
	a.activeTab = 1
	for _, w := range []int{40, 80, 120, 200} {
		view := StatusLine{}.SetHints(a.listHints()).SetWidth(w).View()
		if h := lipgloss.Height(view); h != 1 {
			t.Errorf("width %d: hint line takes %d rows", w, h)
		}
		if lipgloss.Width(view) > w {
			t.Errorf("width %d: hint line is %d wide", w, lipgloss.Width(view))
		}
	}
}
