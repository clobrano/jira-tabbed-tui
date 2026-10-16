package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// jiraHome points HOME at a jira-cli config with the given installation
// type and sets an API token, so SearchPages talks to srvURL.
func jiraHome(t *testing.T, srvURL, installation string, token bool) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".config", ".jira")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("server: %s\nlogin: me@example.com\ninstallation: %s\n", srvURL, installation)
	if err := os.WriteFile(filepath.Join(dir, ".config.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	if token {
		t.Setenv("JIRA_API_TOKEN", "tok")
	} else {
		t.Setenv("JIRA_API_TOKEN", "")
	}
}

func issuesJSON(from, n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range out {
		out[i] = map[string]any{"key": fmt.Sprintf("P-%d", from+i+1),
			"fields": map[string]any{"summary": "s", "status": map[string]any{"name": "To Do"}}}
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }

func TestSearchPagesCloud(t *testing.T) {
	var gotFields, gotToken []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/search/jql":
			q := r.URL.Query()
			gotFields = append(gotFields, q.Get("fields"))
			gotToken = append(gotToken, q.Get("nextPageToken"))
			if q.Get("maxResults") != "100" {
				t.Errorf("maxResults = %q", q.Get("maxResults"))
			}
			switch q.Get("nextPageToken") {
			case "":
				writeJSON(w, map[string]any{"issues": issuesJSON(0, 100), "nextPageToken": "t2", "isLast": false})
			case "t2":
				writeJSON(w, map[string]any{"issues": issuesJSON(100, 50), "isLast": true})
			}
		case "/rest/api/3/search/approximate-count":
			if r.Method != http.MethodPost {
				t.Errorf("approximate-count method %s", r.Method)
			}
			writeJSON(w, map[string]int{"count": 150})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	jiraHome(t, srv.URL, "Cloud", true)

	fetch := SearchPages(NewFakeRunner(), "", []string{"summary", "status", "customfield_1"})
	p1, err := fetch("project = P", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Issues) != 100 || p1.Total != 150 || p1.Next != "t2" {
		t.Fatalf("page 1: %d issues, total %d, next %q", len(p1.Issues), p1.Total, p1.Next)
	}
	p2, err := fetch("project = P", p1.Next)
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Issues) != 50 || p2.Next != "" || p2.Issues[0].Key != "P-101" {
		t.Fatalf("page 2: %d issues, next %q, first %s", len(p2.Issues), p2.Next, p2.Issues[0].Key)
	}
	if gotFields[0] != "summary,status,customfield_1" || strings.Join(gotToken, ",") != ",t2" {
		t.Errorf("fields %q, tokens %q", gotFields, gotToken)
	}
}

func TestSearchPagesServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/search" {
			http.NotFound(w, r)
			return
		}
		start, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		n := min(100, 230-start)
		writeJSON(w, map[string]any{"startAt": start, "maxResults": 100, "total": 230, "issues": issuesJSON(start, n)})
	}))
	defer srv.Close()
	jiraHome(t, srv.URL, "Local", true)

	fetch := SearchPages(NewFakeRunner(), "", ListFields)
	var keys []string
	cursor, pages := "", 0
	for {
		p, err := fetch("project = P", cursor)
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 230 {
			t.Errorf("total %d", p.Total)
		}
		for _, iss := range p.Issues {
			keys = append(keys, iss.Key)
		}
		pages++
		if cursor = p.Next; cursor == "" {
			break
		}
	}
	if pages != 3 || len(keys) != 230 || keys[229] != "P-230" {
		t.Errorf("%d pages, %d issues, last %s", pages, len(keys), keys[len(keys)-1])
	}
}

func TestSearchPagesFallsBackToJiraCLIWithoutToken(t *testing.T) {
	jiraHome(t, "https://jira.example.com", "Cloud", false)
	t.Setenv("PATH", "") // no secret-tool either
	r := NewFakeRunner()
	data, err := os.ReadFile(filepath.Join("testdata", "list.json"))
	if err != nil {
		t.Fatal(err)
	}
	r.Register(data, "issue", "list", "--raw", "-q", "project = P", "--paginate", "0:100")

	p, err := SearchPages(r, "", ListFields)("project = P", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Issues) == 0 || p.Next != "" || p.Truncated {
		t.Errorf("fallback page: %d issues, next %q, truncated %v", len(p.Issues), p.Next, p.Truncated)
	}
}
