package backend

import (
	"fmt"
	"io"
	"net/http"
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
