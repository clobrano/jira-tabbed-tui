package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/clobrano/jira-tabbed-tui/model"
)

// SearchPageSize is how many issues one list request asks for: the maximum
// both the Jira search API and jira-cli accept.
const SearchPageSize = 100

// SearchPage is one page of a list query.
type SearchPage struct {
	Issues []model.Issue
	// Total is the number of matching issues if known (0 = unknown). On Jira
	// Cloud it comes from the approximate-count API and is only set on the
	// first page.
	Total int
	// Next is the cursor for the following page ("" = this was the last).
	Next string
	// Truncated is set when the REST API couldn't be used and jira-cli
	// returned a full page: more issues may exist but can't be fetched.
	Truncated bool
}

// PageFunc fetches the page of jql that starts at cursor ("" = first page).
type PageFunc func(jql, cursor string) (SearchPage, error)

// ListFields are the issue fields list rows are built from; columns add
// their own on top (see SearchPages).
var ListFields = []string{"summary", "issuetype", "priority", "status", "assignee", "duedate", "created", "updated"}

// SearchPages returns a PageFunc for list queries. It uses the Jira REST
// API, which can page through every result and asks only for the given
// fields: /rest/api/3/search/jql (with nextPageToken) on Jira Cloud and
// /rest/api/2/search (with startAt) on Server/Data Center. Without an API
// token it falls back to a single jira-cli call, which on Jira Cloud can
// neither skip ahead nor report a total, so it returns at most one page.
func SearchPages(r Runner, cfgURL string, fields []string) PageFunc {
	return func(jql, cursor string) (SearchPage, error) {
		baseURL, email, token, err := resolveJiraCredentials(cfgURL)
		if err != nil {
			if cursor != "" {
				return SearchPage{}, nil
			}
			issues, _, err := FetchIssueList(r, jql, SearchPageSize)
			return SearchPage{Issues: issues, Total: len(issues), Truncated: len(issues) >= SearchPageSize}, err
		}
		c := searchClient{baseURL: baseURL, email: email, token: token}
		if isJiraCloud() {
			return c.cloudPage(jql, cursor, fields)
		}
		return c.serverPage(jql, cursor, fields)
	}
}

type searchClient struct{ baseURL, email, token string }

func (c searchClient) do(method, path string, body any, dest any) error {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rd)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jira API %s %s returned %d: %s", method, strings.SplitN(path, "?", 2)[0],
			resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, dest)
}

// cloudPage uses the token-based /rest/api/3/search/jql; the first page also
// asks /rest/api/3/search/approximate-count for the total (best effort).
func (c searchClient) cloudPage(jql, cursor string, fields []string) (SearchPage, error) {
	q := url.Values{}
	q.Set("jql", jql)
	q.Set("maxResults", strconv.Itoa(SearchPageSize))
	q.Set("fields", strings.Join(fields, ","))
	if cursor != "" {
		q.Set("nextPageToken", cursor)
	}
	var resp struct {
		Issues        []rawIssue `json:"issues"`
		NextPageToken string     `json:"nextPageToken"`
		IsLast        bool       `json:"isLast"`
	}
	if err := c.do(http.MethodGet, "/rest/api/3/search/jql?"+q.Encode(), nil, &resp); err != nil {
		return SearchPage{}, err
	}
	page := SearchPage{Issues: convertIssues(resp.Issues)}
	if !resp.IsLast {
		page.Next = resp.NextPageToken
	}
	if cursor == "" {
		if page.Next == "" {
			page.Total = len(page.Issues)
		} else {
			var count struct {
				Count int `json:"count"`
			}
			if c.do(http.MethodPost, "/rest/api/3/search/approximate-count", map[string]string{"jql": jql}, &count) == nil {
				page.Total = count.Count
			}
		}
	}
	return page, nil
}

// serverPage uses /rest/api/2/search, which pages with startAt and reports
// the total; the cursor is the next startAt.
func (c searchClient) serverPage(jql, cursor string, fields []string) (SearchPage, error) {
	start := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil {
			return SearchPage{}, fmt.Errorf("bad page cursor %q", cursor)
		}
		start = n
	}
	q := url.Values{}
	q.Set("jql", jql)
	q.Set("startAt", strconv.Itoa(start))
	q.Set("maxResults", strconv.Itoa(SearchPageSize))
	q.Set("fields", strings.Join(fields, ","))
	var resp listResponse
	if err := c.do(http.MethodGet, "/rest/api/2/search?"+q.Encode(), nil, &resp); err != nil {
		return SearchPage{}, err
	}
	page := SearchPage{Issues: convertIssues(resp.Issues), Total: resp.Total}
	if next := start + len(resp.Issues); len(resp.Issues) > 0 && next < resp.Total {
		page.Next = strconv.Itoa(next)
	}
	return page, nil
}
