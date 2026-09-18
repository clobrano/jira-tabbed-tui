package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Backend struct {
	CLI       string   `yaml:"cli"`
	ExtraArgs []string `yaml:"extra_args"`
	URL       string   `yaml:"url"` // Jira base URL for browser links, e.g. https://myorg.atlassian.net
}

type Tab struct {
	Name string `yaml:"name"`
	JQL  string `yaml:"jql"`
}

type ListColumn struct {
	Field string `yaml:"field"`
	Label string `yaml:"label"`
	Width int    `yaml:"width"` // 0 = flexible (only one column should be flexible)
}

type List struct {
	Columns []ListColumn `yaml:"columns"`
}

type SidebarField struct {
	Field string `yaml:"field"`
	Label string `yaml:"label"`
}

type Detail struct {
	SidebarWidth  int            `yaml:"sidebar_width"`
	SidebarFields []SidebarField `yaml:"sidebar_fields"`
}

type Keybindings struct {
	Transition    string `yaml:"transition"`
	AddLabels     string `yaml:"add_labels"`
	AddComment    string `yaml:"add_comment"`
	OpenBrowser   string `yaml:"open_browser"`
	FieldDiscover string `yaml:"field_discovery"`
	ForceRefresh  string `yaml:"force_refresh"`
	Help          string `yaml:"help"`
}

type Config struct {
	Backend     Backend     `yaml:"backend"`
	Tabs        []Tab       `yaml:"tabs"`
	List        List        `yaml:"list"`
	Detail      Detail      `yaml:"detail"`
	Keybindings Keybindings `yaml:"keybindings"`
}

// Save writes cfg back to path in YAML format.
func Save(path string, cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// DefaultPath returns the default config file path.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "jira-tui", "config.yaml")
}

// defaultConfigYAML is written to disk when no config file exists.
const defaultConfigYAML = `backend:
  cli: jira
  # url: https://myorg.atlassian.net   # base URL for "open in browser"; omit to use 'jira open KEY'
  # extra_args: []

tabs:
  - name: Assigned
    jql: assignee = currentUser()
  - name: In Progress
    jql: assignee = currentUser() AND status = "In Progress"

list:
  # Available fields: key, type, summary, priority, status, duedate
  # label overrides the column header; width fixes the column width in chars
  # (omit width on exactly one column to make it expand to fill remaining space).
  columns:
    - field: key
      label: ID
    - field: type
    - field: summary
    - field: status

detail:
  sidebar_width: 33
  sidebar_fields:
    - field: assignee
    - field: reporter
    - field: labels
    - field: duedate
      label: Due
`

// Load reads and validates a config file at path.
// If the file does not exist it is created with sane defaults.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("reading config: %w", err)
		}
		// Create the file (and any parent directories) with defaults.
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return Config{}, fmt.Errorf("creating config directory: %w", err)
		}
		if err := os.WriteFile(path, []byte(defaultConfigYAML), 0644); err != nil {
			return Config{}, fmt.Errorf("writing default config to %s: %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "Created default config: %s\n", path)
		data = []byte(defaultConfigYAML)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Backend.CLI == "" {
		c.Backend.CLI = "jira"
	}
	if len(c.List.Columns) == 0 {
		c.List.Columns = []ListColumn{
			{Field: "key"},
			{Field: "type"},
			{Field: "summary"},
			{Field: "status"},
		}
	}
	if c.Detail.SidebarWidth == 0 {
		c.Detail.SidebarWidth = 33
	}
	if len(c.Detail.SidebarFields) == 0 {
		c.Detail.SidebarFields = []SidebarField{
			{Field: "assignee"},
			{Field: "reporter"},
			{Field: "labels"},
			{Field: "duedate"},
		}
	}
	kb := &c.Keybindings
	if kb.Transition == "" {
		kb.Transition = "s"
	}
	if kb.AddLabels == "" {
		kb.AddLabels = "l"
	}
	if kb.AddComment == "" {
		kb.AddComment = "c"
	}
	if kb.OpenBrowser == "" {
		kb.OpenBrowser = "o"
	}
	if kb.FieldDiscover == "" {
		kb.FieldDiscover = "F"
	}
	if kb.ForceRefresh == "" {
		kb.ForceRefresh = "r"
	}
	if kb.Help == "" {
		kb.Help = "?"
	}
}

// knownBuiltinFields are fields with canonical names (not custom fields).
var knownBuiltinFields = map[string]bool{
	"assignee": true, "reporter": true, "labels": true,
	"duedate": true, "priority": true, "status": true,
	"summary": true, "issuetype": true, "description": true,
	"created": true, "updated": true,
}

func (c *Config) validate() error {
	for i, tab := range c.Tabs {
		if tab.JQL == "" {
			return fmt.Errorf("tab %d (%q) is missing a jql value", i+1, tab.Name)
		}
	}
	for _, sf := range c.Detail.SidebarFields {
		isCustom := len(sf.Field) > 12 && sf.Field[:12] == "customfield_"
		if !knownBuiltinFields[sf.Field] && !isCustom {
			fmt.Fprintf(os.Stderr, "warning: unknown sidebar field %q\n", sf.Field)
		}
	}
	return nil
}
