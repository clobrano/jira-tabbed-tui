package tui

import (
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
	"github.com/clobrano/jira-tabbed-tui/tui/actions"
)

// clipboardDoneMsg is sent after text has been written to the clipboard.
type clipboardDoneMsg struct {
	what string // human-readable description of what was copied
	osc  bool   // true when the OSC 52 terminal fallback was used
}

// copyToClipboardCmd writes text to the system clipboard (xclip/xsel/wl-copy/
// pbcopy). When no clipboard tool is available — e.g. over SSH — it falls back
// to the OSC 52 escape sequence, which most modern terminals honour.
func copyToClipboardCmd(text, what string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteAll(text); err == nil {
			return clipboardDoneMsg{what: what}
		}
		termenv.Copy(text)
		return clipboardDoneMsg{what: what, osc: true}
	}
}

// urlCopyItem is the "full URL to the ticket" entry, copied directly with "u".
func urlCopyItem(cfgURL, key string) (actions.CopyItem, bool) {
	url := backend.IssueURL(cfgURL, key)
	if url == "" {
		return actions.CopyItem{}, false
	}
	return actions.CopyItem{Label: "URL", Value: url, Hotkey: "u"}, true
}

// listCopyItems returns one entry per displayed column (so 1, 2, … match the
// column order) followed by the issue URL.
func listCopyItems(columns []config.ListColumn, iss model.Issue, cfgURL string) []actions.CopyItem {
	items := make([]actions.CopyItem, 0, len(columns)+1)
	for _, c := range columns {
		items = append(items, actions.CopyItem{Label: colLabel(c), Value: issueFieldValue(iss, c.Field)})
	}
	if it, ok := urlCopyItem(cfgURL, iss.Key); ok {
		items = append(items, it)
	}
	return items
}

// detailCopyItems returns the issue's core fields, the configured sidebar
// fields and the description, followed by the issue URL. Empty fields are
// skipped. When a link is highlighted on the Links tab, its key/URL are
// offered too.
func detailCopyItems(d model.IssueDetail, sidebar []config.SidebarField, link *model.IssueLink, cfgURL string) []actions.CopyItem {
	var items []actions.CopyItem
	add := func(label, value string) {
		if value != "" {
			items = append(items, actions.CopyItem{Label: label, Value: value})
		}
	}

	add("Key", d.Key)
	add("Summary", d.Summary)
	add("Type", d.Type)
	add("Status", d.Status)
	add("Priority", d.Priority)
	add("Assignee", d.Assignee)
	add("Due Date", d.DueDate)
	add("Created", d.Created)
	add("Updated", d.Updated)

	// Sidebar fields already covered by the core fields above.
	core := map[string]bool{
		"key": true, "summary": true, "issuetype": true, "status": true, "priority": true,
		"assignee": true, "duedate": true, "created": true, "updated": true,
	}
	for _, sf := range sidebar {
		if core[sf.Field] {
			continue
		}
		core[sf.Field] = true
		label := sf.Label
		if label == "" {
			label = fieldDisplayLabel(sf.Field)
		}
		add(label, fieldValue(d, sf.Field))
	}

	add("Description", d.Description)

	if link != nil {
		if link.URL != "" {
			add("Link URL", link.URL)
		} else {
			add("Linked Key", link.Key)
			add("Linked URL", backend.IssueURL(cfgURL, link.Key))
		}
	}

	if it, ok := urlCopyItem(cfgURL, d.Key); ok {
		items = append(items, it)
	}
	return items
}
