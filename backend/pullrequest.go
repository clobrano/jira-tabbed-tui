package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Pull request states shown on the Links tab.
const (
	PROpen   = "open"
	PRDraft  = "draft"
	PRMerged = "merged"
	PRClosed = "closed"
)

var (
	githubPRRe = regexp.MustCompile(`^https?://([^/]+)/([^/]+)/([^/]+)/pull/(\d+)`)
	gitlabMRRe = regexp.MustCompile(`^https?://([^/]+)/(.+?)/-/merge_requests/(\d+)`)
)

// prAPI describes how to query one pull/merge request.
type prAPI struct {
	endpoint string
	gitlab   bool
}

// IsPullRequestURL reports whether url points to a GitHub pull request or a
// GitLab merge request whose state can be looked up.
func IsPullRequestURL(u string) bool {
	_, ok := pullRequestAPI(u)
	return ok
}

// pullRequestAPI maps a pull/merge request web URL to its REST endpoint.
//
// GitHub: github.com links use https://api.github.com, or $GITHUB_API_URL when
// set (as GitHub Actions does, and for GitHub Enterprise); other hosts named
// github.* are treated as GitHub Enterprise (https://HOST/api/v3).
// GitLab: any host with a /-/merge_requests/N path (https://HOST/api/v4).
func pullRequestAPI(u string) (prAPI, bool) {
	if m := githubPRRe.FindStringSubmatch(u); m != nil {
		host, owner, repo, num := m[1], m[2], m[3], m[4]
		var base string
		switch {
		case host == "github.com":
			base = strings.TrimRight(os.Getenv("GITHUB_API_URL"), "/")
			if base == "" {
				base = "https://api.github.com"
			}
		case strings.HasPrefix(host, "github."):
			base = "https://" + host + "/api/v3"
		default:
			return prAPI{}, false
		}
		return prAPI{endpoint: fmt.Sprintf("%s/repos/%s/%s/pulls/%s", base, owner, repo, num)}, true
	}
	if m := gitlabMRRe.FindStringSubmatch(u); m != nil {
		host, project, num := m[1], m[2], m[3]
		return prAPI{
			endpoint: fmt.Sprintf("https://%s/api/v4/projects/%s/merge_requests/%s", host, url.PathEscape(project), num),
			gitlab:   true,
		}, true
	}
	return prAPI{}, false
}

var (
	prClient = &http.Client{Timeout: 10 * time.Second}

	prCacheMu sync.Mutex
	prCache   = map[string]prCacheEntry{}
)

type prCacheEntry struct {
	state     string
	fetchedAt time.Time
}

const prCacheTTL = 5 * time.Minute

// FetchPullRequestState returns the state of a pull/merge request: one of
// PROpen, PRDraft, PRMerged or PRClosed. Results are cached for a few minutes
// so moving back and forth between issues does not hit API rate limits.
//
// Requests are anonymous unless GITHUB_TOKEN / GH_TOKEN (GitHub) or
// GITLAB_TOKEN (GitLab) is set; private repositories need a token.
func FetchPullRequestState(u string) (string, error) {
	api, ok := pullRequestAPI(u)
	if !ok {
		return "", fmt.Errorf("not a pull request URL: %s", u)
	}

	prCacheMu.Lock()
	if e, ok := prCache[api.endpoint]; ok && time.Since(e.fetchedAt) < prCacheTTL {
		prCacheMu.Unlock()
		return e.state, nil
	}
	prCacheMu.Unlock()

	req, err := http.NewRequest(http.MethodGet, api.endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	if api.gitlab {
		if tok := os.Getenv("GITLAB_TOKEN"); tok != "" {
			req.Header.Set("PRIVATE-TOKEN", tok)
		}
	} else {
		tok := os.Getenv("GITHUB_TOKEN")
		if tok == "" {
			tok = os.Getenv("GH_TOKEN")
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}

	resp, err := prClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned %d", api.endpoint, resp.StatusCode)
	}

	var raw struct {
		State  string `json:"state"`
		Merged bool   `json:"merged"` // GitHub
		Draft  bool   `json:"draft"`  // GitHub and GitLab
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", fmt.Errorf("parsing %s: %w", api.endpoint, err)
	}
	state := prState(raw.State, raw.Merged, raw.Draft)

	prCacheMu.Lock()
	prCache[api.endpoint] = prCacheEntry{state: state, fetchedAt: time.Now()}
	prCacheMu.Unlock()
	return state, nil
}

// prState normalises GitHub ("open"/"closed" + merged) and GitLab
// ("opened"/"merged"/"closed"/"locked") states.
func prState(state string, merged, draft bool) string {
	switch {
	case merged || state == "merged":
		return PRMerged
	case state == "closed" || state == "locked":
		return PRClosed
	case draft:
		return PRDraft
	default:
		return PROpen
	}
}

// PullRequestStatesFetchedMsg carries the states of an issue's PR links,
// keyed by URL. Links whose lookup failed are absent.
type PullRequestStatesFetchedMsg struct {
	IssueKey string
	States   map[string]string
}

// FetchPullRequestStatesCmd looks up the given PR URLs concurrently.
func FetchPullRequestStatesCmd(issueKey string, urls []string) tea.Cmd {
	return func() tea.Msg {
		states := make(map[string]string, len(urls))
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, u := range urls {
			wg.Add(1)
			go func(u string) {
				defer wg.Done()
				if s, err := FetchPullRequestState(u); err == nil {
					mu.Lock()
					states[u] = s
					mu.Unlock()
				}
			}(u)
		}
		wg.Wait()
		return PullRequestStatesFetchedMsg{IssueKey: issueKey, States: states}
	}
}
