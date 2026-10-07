package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
)

func TestWrapValue(t *testing.T) {
	cases := []struct {
		value string
		w     int
		want  []string
	}{
		// Lists break between items, keeping the comma on the line it ends.
		{"OSAC, OSAC-CLI, OSAC-UI, OSAC-API", 16, []string{"OSAC, OSAC-CLI,", "OSAC-UI,", "OSAC-API"}},
		// An item with spaces stays whole when it fits a line.
		{"Core, Core Infrastructure", 20, []string{"Core,", "Core Infrastructure"}},
		// An item wider than a line is word-wrapped on its own.
		{"Core, Very Long Component Name", 10, []string{"Core,", "Very Long", "Component", "Name"}},
		// Plain text word-wraps; a single long word is hard-broken.
		{"Login flickers on Safari", 12, []string{"Login", "flickers on", "Safari"}},
		{"2026-09-08T14:17:34.000+0000", 12, []string{"2026-09-08T1", "4:17:34.000+", "0000"}},
		{"short", 20, []string{"short"}},
	}
	for _, c := range cases {
		if got := wrapValue(c.value, c.w); !reflect.DeepEqual(got, c.want) {
			t.Errorf("wrapValue(%q, %d) = %q, want %q", c.value, c.w, got, c.want)
		}
	}
}

func sidebarFixture() (Sidebar, model.IssueDetail) {
	sb := NewSidebar([]config.SidebarField{
		{Field: "labels"}, {Field: "components"}, {Field: "customfield_10470", Label: "QA Contact(cf_10470)"},
	})
	d := model.IssueDetail{Issue: model.Issue{Fields: map[string]any{
		"labels":     []any{"OSAC", "OSAC-CLI", "OSAC-UI", "OSAC-API"},
		"components": []any{map[string]any{"name": "Core"}, map[string]any{"name": "Infrastructure"}},
	}}}
	return sb, d
}

func TestSidebarWrapsInsteadOfTruncating(t *testing.T) {
	sb, d := sidebarFixture()
	view := stripANSI(sb.SetWidth(22).View(d))
	if strings.Contains(view, "…") {
		t.Errorf("sidebar still truncates:\n%s", view)
	}
	for _, want := range []string{"OSAC-API", "Infrastructure", "QA", "Contact(cf_10470):"} {
		if !strings.Contains(view, want) {
			t.Errorf("sidebar lost %q:\n%s", want, view)
		}
	}
	// 22 columns plus the left border, which the detail layout reserves.
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 23 {
			t.Errorf("line wider than the sidebar (%d): %q", lipgloss.Width(line), line)
		}
	}
}

func TestSidebarFitsItsHeight(t *testing.T) {
	sb, d := sidebarFixture()
	view := stripANSI(sb.SetWidth(22).SetHeight(6).View(d))
	if h := lipgloss.Height(view); h > 6 {
		t.Fatalf("sidebar is %d rows, limit 6:\n%s", h, view)
	}
	// Only whole fields are shown; the rest are counted.
	if !strings.Contains(view, "… 2 more fields") || strings.Contains(view, "Components") {
		t.Errorf("clamped sidebar:\n%s", view)
	}
}
