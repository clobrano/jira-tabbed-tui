package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"github.com/clobrano/jira-tabbed-tui/model"
)

type jiraCLIConfigFile struct {
	Server string `yaml:"server"`
	Login  string `yaml:"login"`
}

// resolveJiraCredentials returns the Jira base URL, login email, and API token.
// cfgURL (from our config) overrides the server found in jira-cli's config file.
// Token is read from JIRA_API_TOKEN env var, then via secret-tool (GNOME keyring).
func resolveJiraCredentials(cfgURL string) (baseURL, email, token string, err error) {
	// Read jira-cli's config for server + login.
	if home, herr := os.UserHomeDir(); herr == nil {
		data, ferr := os.ReadFile(filepath.Join(home, ".config", ".jira", ".config.yml"))
		if ferr == nil {
			var f jiraCLIConfigFile
			if yaml.Unmarshal(data, &f) == nil {
				baseURL = strings.TrimRight(f.Server, "/")
				email = f.Login
			}
		}
	}
	if cfgURL != "" {
		baseURL = strings.TrimRight(cfgURL, "/")
	}
	// Ensure the URL has a scheme so net/http can parse it.
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	if baseURL == "" {
		err = fmt.Errorf("jira server URL not configured; set backend.url in config or run 'jira init'")
		return
	}

	// Token: env var first.
	token = os.Getenv("JIRA_API_TOKEN")

	// Fall back to GNOME keyring via secret-tool.
	if token == "" && email != "" {
		out, serr := exec.Command("secret-tool", "lookup", "service", "jira-cli", "account", email).Output()
		if serr == nil {
			token = strings.TrimSpace(string(out))
		}
	}

	if token == "" {
		err = fmt.Errorf("jira API token not found; set the JIRA_API_TOKEN environment variable")
	}
	return
}

// FetchTransitionsREST calls the Jira REST API directly to list transitions for key.
func FetchTransitionsREST(cfgURL, key string) ([]model.Transition, error) {
	baseURL, email, token, err := resolveJiraCredentials(cfgURL)
	if err != nil {
		return nil, err
	}

	url := baseURL + "/rest/api/3/issue/" + key + "/transitions"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching transitions: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira API returned %d: %s", resp.StatusCode, body)
	}

	return parseTransitions(body)
}

// SearchAssignableUsersREST searches for users who can be assigned to issueKey.
func SearchAssignableUsersREST(cfgURL, issueKey, query string) ([]model.User, error) {
	baseURL, email, token, err := resolveJiraCredentials(cfgURL)
	if err != nil {
		return nil, err
	}

	endpoint := baseURL + "/rest/api/3/user/assignable/search?issueKey=" +
		url.QueryEscape(issueKey) + "&query=" + url.QueryEscape(query) + "&maxResults=15"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching users: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira API returned %d: %s", resp.StatusCode, body)
	}

	var raw []struct {
		AccountID    string `json:"accountId"`
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing user search: %w", err)
	}
	users := make([]model.User, 0, len(raw))
	for _, u := range raw {
		users = append(users, model.User{
			AccountID:   u.AccountID,
			DisplayName: u.DisplayName,
			Email:       u.EmailAddress,
		})
	}
	return users, nil
}

// FetchPrioritiesREST returns the list of available priorities.
func FetchPrioritiesREST(cfgURL string) ([]string, error) {
	baseURL, email, token, err := resolveJiraCredentials(cfgURL)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := restGetJSON(baseURL, email, token, "/rest/api/3/priority", &raw); err != nil {
		return nil, fmt.Errorf("fetching priorities: %w", err)
	}
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		out = append(out, p.Name)
	}
	return out, nil
}

// FetchCustomFieldOptionsREST returns allowed option values for a custom select field.
// Returns nil, nil when the field has no enumerable options (caller should fall back to text input).
func FetchCustomFieldOptionsREST(cfgURL, fieldID string) ([]string, error) {
	baseURL, email, token, err := resolveJiraCredentials(cfgURL)
	if err != nil {
		return nil, err
	}
	// Step 1: get contexts for this field.
	var ctxResp struct {
		Values []struct{ ID string `json:"id"` } `json:"values"`
	}
	if err := restGetJSON(baseURL, email, token,
		"/rest/api/3/field/"+url.PathEscape(fieldID)+"/context?maxResults=1", &ctxResp); err != nil {
		return nil, nil // not a managed field
	}
	if len(ctxResp.Values) == 0 {
		return nil, nil
	}
	ctxID := ctxResp.Values[0].ID

	// Step 2: get options for the first context.
	var optResp struct {
		Values []struct{ Value string `json:"value"` } `json:"values"`
	}
	if err := restGetJSON(baseURL, email, token,
		"/rest/api/3/field/"+url.PathEscape(fieldID)+"/context/"+ctxID+"/option?maxResults=50",
		&optResp); err != nil {
		return nil, nil // not a select field
	}
	out := make([]string, 0, len(optResp.Values))
	for _, o := range optResp.Values {
		out = append(out, o.Value)
	}
	return out, nil
}

// projectKeyFromIssueKey extracts the project key from an issue key (e.g. "PROJ" from "PROJ-123").
func projectKeyFromIssueKey(issueKey string) string {
	if i := strings.LastIndex(issueKey, "-"); i > 0 {
		return issueKey[:i]
	}
	return issueKey
}

// restGetJSON performs an authenticated GET and unmarshals the response into dest.
func restGetJSON(baseURL, email, token, path string, dest any) error {
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jira API %s returned %d", path, resp.StatusCode)
	}
	return json.Unmarshal(body, dest)
}

// FetchProjectVersionsREST returns the release versions defined for the project.
func FetchProjectVersionsREST(cfgURL, issueKey string) ([]string, error) {
	baseURL, email, token, err := resolveJiraCredentials(cfgURL)
	if err != nil {
		return nil, err
	}
	projectKey := projectKeyFromIssueKey(issueKey)
	var raw []struct {
		Name string `json:"name"`
	}
	if err := restGetJSON(baseURL, email, token,
		"/rest/api/3/project/"+url.PathEscape(projectKey)+"/versions", &raw); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if v.Name != "" {
			out = append(out, v.Name)
		}
	}
	return out, nil
}

// FetchProjectComponentsREST returns the components defined for the project.
func FetchProjectComponentsREST(cfgURL, issueKey string) ([]string, error) {
	baseURL, email, token, err := resolveJiraCredentials(cfgURL)
	if err != nil {
		return nil, err
	}
	projectKey := projectKeyFromIssueKey(issueKey)
	var raw []struct {
		Name string `json:"name"`
	}
	if err := restGetJSON(baseURL, email, token,
		"/rest/api/3/project/"+url.PathEscape(projectKey)+"/components", &raw); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if v.Name != "" {
			out = append(out, v.Name)
		}
	}
	return out, nil
}

// FetchIssueTypesREST returns all issue types available in the system.
func FetchIssueTypesREST(cfgURL string) ([]string, error) {
	baseURL, email, token, err := resolveJiraCredentials(cfgURL)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := restGetJSON(baseURL, email, token, "/rest/api/3/issuetype", &raw); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if v.Name != "" {
			out = append(out, v.Name)
		}
	}
	return out, nil
}

// FetchRemoteLinksREST fetches web/remote links attached to an issue.
func FetchRemoteLinksREST(cfgURL, key string) ([]model.IssueLink, error) {
	baseURL, email, token, err := resolveJiraCredentials(cfgURL)
	if err != nil {
		return nil, err
	}

	endpoint := baseURL + "/rest/api/3/issue/" + url.PathEscape(key) + "/remotelink"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching remote links: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira API returned %d: %s", resp.StatusCode, body)
	}

	var raw []struct {
		Relationship string `json:"relationship"`
		Object       struct {
			URL     string `json:"url"`
			Title   string `json:"title"`
			Summary string `json:"summary"`
		} `json:"object"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing remote links: %w", err)
	}

	links := make([]model.IssueLink, 0, len(raw))
	for _, r := range raw {
		rel := r.Relationship
		if rel == "" {
			rel = "web link"
		}
		links = append(links, model.IssueLink{
			Type:    rel,
			Key:     r.Object.Title,
			Summary: r.Object.URL,
			URL:     r.Object.URL,
		})
	}
	return links, nil
}
