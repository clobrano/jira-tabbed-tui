package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
	"github.com/clobrano/jira-tabbed-tui/tui/actions"
)

func TestListCopyItemsFollowColumnsThenURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cols := []config.ListColumn{{Field: "key"}, {Field: "summary", Label: "Title"}, {Field: "status"}}
	iss := model.Issue{Key: "PROJ-1", Summary: "Fix it", Status: "Open"}

	items := listCopyItems(cols, iss, "jira.example.com/")
	want := []actions.CopyItem{
		{Label: "Key", Value: "PROJ-1"},
		{Label: "Title", Value: "Fix it"},
		{Label: "Status", Value: "Open"},
		{Label: "URL", Value: "https://jira.example.com/browse/PROJ-1", Hotkey: "u"},
	}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d: %+v", len(items), len(want), items)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, items[i], want[i])
		}
	}
}

func TestListCopyItemsWithoutServerHasNoURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	items := listCopyItems([]config.ListColumn{{Field: "key"}}, model.Issue{Key: "PROJ-1"}, "")
	if len(items) != 1 || items[0].Label != "Key" {
		t.Fatalf("got %+v, want only the key column", items)
	}
}

func TestDetailCopyItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := model.IssueDetail{
		Issue: model.Issue{
			Key: "PROJ-1", Summary: "Fix it", Status: "Open",
			Fields: map[string]any{
				"assignee": map[string]any{"displayName": "Ann"},
				"labels":   []any{"a", "b"},
			},
			Assignee: "Ann",
		},
		Description: "Body",
	}
	sidebar := []config.SidebarField{{Field: "assignee"}, {Field: "labels"}, {Field: "reporter"}}
	link := &model.IssueLink{Key: "PROJ-2"}

	items := detailCopyItems(d, sidebar, link, "https://jira.example.com")
	got := map[string]string{}
	var order []string
	for _, it := range items {
		got[it.Label] = it.Value
		order = append(order, it.Label)
	}
	want := map[string]string{
		"Key":         "PROJ-1",
		"Summary":     "Fix it",
		"Status":      "Open",
		"Assignee":    "Ann",
		"Labels":      "a, b",
		"Description": "Body",
		"Linked Key":  "PROJ-2",
		"Linked URL":  "https://jira.example.com/browse/PROJ-2",
		"URL":         "https://jira.example.com/browse/PROJ-1",
	}
	if len(got) != len(want) || len(items) != len(want) {
		t.Fatalf("labels = %v, want %d unique entries (empty fields skipped, assignee not duplicated)", order, len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if order[len(order)-1] != "URL" {
		t.Errorf("last item = %q, want URL", order[len(order)-1])
	}
}

func runCopyKeys(t *testing.T, m actions.CopyModel, keys ...string) actions.CopyConfirmedMsg {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case " ":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m, cmd = m.Update(msg)
	}
	if cmd == nil {
		t.Fatalf("keys %q produced no command", keys)
	}
	got, ok := cmd().(actions.CopyConfirmedMsg)
	if !ok {
		t.Fatalf("keys %q did not confirm a copy", keys)
	}
	return got
}

func TestCopyModelSelection(t *testing.T) {
	items := []actions.CopyItem{
		{Label: "Key", Value: "PROJ-1"},
		{Label: "Summary", Value: "Fix it"},
		{Label: "URL", Value: "https://x/browse/PROJ-1", Hotkey: "u"},
	}
	m := actions.NewCopyModel("Copy", items)

	cases := []struct {
		name string
		keys []string
		want string
	}{
		{"digit copies that item", []string{"2"}, "Fix it"},
		{"hotkey copies url", []string{"u"}, "https://x/browse/PROJ-1"},
		{"enter copies highlighted", []string{"j", "enter"}, "Fix it"},
		{"marked items joined by space", []string{" ", "j", " ", "enter"}, "PROJ-1 Fix it"},
		{"tab switches to newline", []string{"a", "tab", "enter"}, "PROJ-1\nFix it\nhttps://x/browse/PROJ-1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runCopyKeys(t, m, c.keys...); got.Text != c.want {
				t.Errorf("Text = %q, want %q", got.Text, c.want)
			}
		})
	}
}
