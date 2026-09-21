package tui

import (
	"testing"

	"github.com/clobrano/jira-tabbed-tui/model"
)

func TestIssueFieldValueUsesRawJiraFields(t *testing.T) {
	issue := model.Issue{
		Fields: map[string]any{
			"fixVersions": []any{
				map[string]any{"id": "10001", "name": "4.18"},
				map[string]any{"id": "10002", "name": "4.19"},
			},
			"customfield_10020": "Sprint 5",
		},
	}

	if got := issueFieldValue(issue, "fixVersions"); got != "4.18, 4.19" {
		t.Errorf("fixVersions = %q, want %q", got, "4.18, 4.19")
	}
	if got := issueFieldValue(issue, "customfield_10020"); got != "Sprint 5" {
		t.Errorf("customfield_10020 = %q, want %q", got, "Sprint 5")
	}
}

func TestIssueFieldValueFormatsParentObject(t *testing.T) {
	issue := model.Issue{
		Fields: map[string]any{
			"parent": map[string]any{
				"key": "OSAC-5427",
				"fields": map[string]any{
					"summary": "SSH Keys resource",
				},
			},
		},
	}

	if got := issueFieldValue(issue, "parent"); got != "OSAC-5427" {
		t.Errorf("parent = %q, want %q", got, "OSAC-5427")
	}
}
