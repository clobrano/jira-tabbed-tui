package backend_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/clobrano/jira-tabbed-tui/backend"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(testdataPath(name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

func TestFetchIssueList(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register(loadFixture(t, "list.json"), "issue", "list", "--raw", "-q", "project = PROJ", "--paginate", "0:50")

	issues, total, err := backend.FetchIssueList(r, "project = PROJ", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].Key != "PROJ-123" {
		t.Errorf("expected first key PROJ-123, got %q", issues[0].Key)
	}
	if issues[0].Type != "Bug" {
		t.Errorf("expected type Bug, got %q", issues[0].Type)
	}
	if issues[0].Priority != "High" {
		t.Errorf("expected priority High, got %q", issues[0].Priority)
	}
	if issues[0].DueDate != "2026-10-01" {
		t.Errorf("expected duedate 2026-10-01, got %q", issues[0].DueDate)
	}
	fixVersions, ok := issues[0].Fields["fixVersions"].([]any)
	if !ok || len(fixVersions) != 1 {
		t.Fatalf("expected one raw fixVersions value, got %#v", issues[0].Fields["fixVersions"])
	}
	if issues[1].DueDate != "" {
		t.Errorf("expected empty duedate for second issue, got %q", issues[1].DueDate)
	}
}

func TestFetchIssueDetail(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register(loadFixture(t, "detail.json"), "issue", "view", "PROJ-123", "--raw")

	detail, err := backend.FetchIssueDetail(r, "PROJ-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Key != "PROJ-123" {
		t.Errorf("expected key PROJ-123, got %q", detail.Key)
	}
	if detail.Description == "" {
		t.Error("expected non-empty description")
	}
	if len(detail.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(detail.Comments))
	}
	// Comments should be newest-first.
	if detail.Comments[0].Author != "Alice Smith" {
		t.Errorf("expected first comment from Alice Smith (newest), got %q", detail.Comments[0].Author)
	}
	if detail.Fields["assignee"] != "Alice Smith" {
		t.Errorf("expected assignee Alice Smith, got %v", detail.Fields["assignee"])
	}
	if detail.Fields["reporter"] != "Bob Jones" {
		t.Errorf("expected reporter Bob Jones, got %v", detail.Fields["reporter"])
	}
}

func TestFetchTransitions(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register(loadFixture(t, "transitions.json"), "issue", "transitions", "PROJ-123", "--raw")

	ts, err := backend.FetchTransitions(r, "PROJ-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ts) != 4 {
		t.Fatalf("expected 4 transitions, got %d", len(ts))
	}
	var current *struct{ name string }
	for _, tr := range ts {
		if tr.IsCurrentStatus {
			current = &struct{ name string }{tr.Name}
		}
	}
	if current == nil || current.name != "In Progress" {
		t.Error("expected 'In Progress' to be the current status")
	}
}

func TestFetchAllFields(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register(loadFixture(t, "custom_fields.json"), "issue", "view", "PROJ-125", "--raw")

	fields, err := backend.FetchAllFields(r, "PROJ-125")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fieldMap := make(map[string]string)
	for _, f := range fields {
		fieldMap[f.ID] = f.DisplayName
	}

	if _, ok := fieldMap["customfield_10020"]; !ok {
		t.Error("expected customfield_10020 to be present")
	}
	if _, ok := fieldMap["customfield_10016"]; !ok {
		t.Error("expected customfield_10016 to be present")
	}
	if fieldMap["assignee"] != "Assignee" {
		t.Errorf("expected assignee display name 'Assignee', got %q", fieldMap["assignee"])
	}
	if fieldMap["summary"] != "Summary" {
		t.Errorf("expected summary display name 'Summary', got %q", fieldMap["summary"])
	}
}

func TestFakeRunnerMissingKey(t *testing.T) {
	r := backend.NewFakeRunner()
	_, err := r.Run("issue", "list", "--raw")
	if err == nil {
		t.Fatal("expected error for unregistered key, got nil")
	}
}

func TestWriteFunctions(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register([]byte(`{"success":true}`), "issue", "move", "PROJ-123", "Done")
	r.Register([]byte(`{"success":true}`), "issue", "label", "add", "PROJ-123", "backend")
	r.Register([]byte(`{"success":true}`), "issue", "comment", "add", "PROJ-123", "hello")

	if err := backend.ApplyTransition(r, "PROJ-123", "Done"); err != nil {
		t.Errorf("ApplyTransition: %v", err)
	}
	if err := backend.AddLabels(r, "PROJ-123", []string{"backend"}); err != nil {
		t.Errorf("AddLabels: %v", err)
	}
	if err := backend.AddComment(r, "PROJ-123", "hello"); err != nil {
		t.Errorf("AddComment: %v", err)
	}
}
