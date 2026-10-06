package backend

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPullRequestAPI(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "")
	cases := []struct {
		url, want string
		gitlab    bool
		ok        bool
	}{
		{"https://github.com/acme/web/pull/42", "https://api.github.com/repos/acme/web/pulls/42", false, true},
		{"https://github.com/acme/web/pull/42/files", "https://api.github.com/repos/acme/web/pulls/42", false, true},
		{"https://github.example.com/acme/web/pull/7", "https://github.example.com/api/v3/repos/acme/web/pulls/7", false, true},
		{"https://gitlab.com/group/sub/proj/-/merge_requests/9", "https://gitlab.com/api/v4/projects/group%2Fsub%2Fproj/merge_requests/9", true, true},
		{"https://github.com/acme/web/issues/42", "", false, false},
		{"https://example.com/acme/web/pull/42", "", false, false},
		{"https://status.example.com/incident", "", false, false},
	}
	for _, c := range cases {
		api, ok := pullRequestAPI(c.url)
		if ok != c.ok || api.endpoint != c.want || api.gitlab != c.gitlab {
			t.Errorf("pullRequestAPI(%q) = %+v, %v; want %q gitlab=%v ok=%v", c.url, api, ok, c.want, c.gitlab, c.ok)
		}
	}
}

func TestPRState(t *testing.T) {
	cases := []struct {
		state         string
		merged, draft bool
		want          string
	}{
		{"open", false, false, PROpen},
		{"open", false, true, PRDraft},
		{"closed", true, false, PRMerged},
		{"closed", false, false, PRClosed},
		{"opened", false, false, PROpen}, // GitLab
		{"merged", false, false, PRMerged},
		{"locked", false, false, PRClosed},
	}
	for _, c := range cases {
		if got := prState(c.state, c.merged, c.draft); got != c.want {
			t.Errorf("prState(%q, %v, %v) = %q, want %q", c.state, c.merged, c.draft, got, c.want)
		}
	}
}

func TestFetchPullRequestStateUsesGitHubAPIURLAndCaches(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/repos/acme/web/pulls/42" || r.Header.Get("Authorization") != "Bearer tok" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"state":"closed","merged":true,"draft":false}`))
	}))
	defer srv.Close()
	t.Setenv("GITHUB_API_URL", srv.URL)
	t.Setenv("GITHUB_TOKEN", "tok")

	for i := 0; i < 2; i++ {
		got, err := FetchPullRequestState("https://github.com/acme/web/pull/42")
		if err != nil || got != PRMerged {
			t.Fatalf("FetchPullRequestState = %q, %v; want merged", got, err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("API hit %d times, want 1 (second lookup cached)", hits.Load())
	}
}

func TestParseIssueDetailLinkIssueTypes(t *testing.T) {
	d, err := parseIssueDetail([]byte(`{"key":"P-1","fields":{
		"subtasks":[{"key":"P-2","fields":{"summary":"s","status":{"name":"To Do"},"issuetype":{"name":"Sub-task"}}}],
		"issuelinks":[{"type":{"inward":"is blocked by","outward":"blocks"},
			"outwardIssue":{"key":"P-3","fields":{"summary":"o","status":{"name":"Done"},"issuetype":{"name":"Story"}}}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Links) != 2 || d.Links[0].IssueType != "Sub-task" || d.Links[1].IssueType != "Story" || d.Links[1].Type != "blocks" {
		t.Fatalf("links = %+v", d.Links)
	}
}
