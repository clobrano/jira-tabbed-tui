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

func TestDetailFooterNamesLinkedIssue(t *testing.T) {
	a := newTestApp(t)
	a.view = viewDetail
	a = a.pushHistory("P-1")
	a = send(t, a, fetched("P-1", "P-2"), keyMsg("right"), keyMsg("right"))

	got := hintsPlain(fitHints(a.detailHints(), 300))
	for _, want := range []string{"j/k next/prev link", "enter open P-2", "m change P-2 status", "a assign P-2", "? all keys"} {
		if !strings.Contains(got, want) {
			t.Errorf("detail footer missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "go back") {
		t.Errorf("no history yet, but footer offers go back: %q", got)
	}

	// After opening the link, going back is offered first.
	a = send(t, a, keyMsg("enter"), fetched("P-2"))
	got = hintsPlain(fitHints(a.detailHints(), 300))
	if !strings.HasPrefix(got, "⌫/ctrl+o go back · ") || !strings.Contains(got, "j/k scroll") {
		t.Errorf("footer after following a link = %q", got)
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
