package tui

import (
	"strings"
	"testing"
)

// "m Move status": the label starts with the key's letter, as a mnemonic.
func TestHelpSaysMoveStatus(t *testing.T) {
	a := newTestApp(t)
	for _, inDetail := range []bool{false, true} {
		view := stripANSI(NewHelpOverlay(a.cfg.Keybindings).SetSize(120, 60).SetContext(inDetail).View())
		if !strings.Contains(view, "Move status") || strings.Contains(view, "Change status") {
			t.Errorf("help (detail=%v) doesn't say Move status:\n%s", inDetail, view)
		}
	}
}

// The edit-JQL entry fits on one line of the overlay.
func TestHelpEditJQLOnOneLine(t *testing.T) {
	a := newTestApp(t)
	view := stripANSI(NewHelpOverlay(a.cfg.Keybindings).SetSize(120, 60).View())
	if !strings.Contains(view, "Edit tab JQL (Search tab: type a query)") {
		t.Errorf("edit JQL entry is missing or wrapped:\n%s", view)
	}
}
