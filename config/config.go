package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

type Backend struct {
	CLI       string   `yaml:"cli"`
	ExtraArgs []string `yaml:"extra_args"`
	URL       string   `yaml:"url"` // Jira base URL for browser links, e.g. https://myorg.atlassian.net
}

type Tab struct {
	Name    string       `yaml:"name"`
	JQL     string       `yaml:"jql"`
	Sort    string       `yaml:"sort"`     // field to sort by (e.g. "priority", "key", "duedate")
	SortAsc bool         `yaml:"sort_asc"` // true = ascending, false = descending (default)
	Columns []ListColumn `yaml:"columns"`  // overrides global list.columns when non-empty
}

type ListColumn struct {
	Field string `yaml:"field"`
	Label string `yaml:"label"`
	Width int    `yaml:"width"` // 0 = flexible (only one column should be flexible)
}

type List struct {
	Columns []ListColumn `yaml:"columns"`
	// MaxIssues caps how many issues a tab loads (default 1000), so a very
	// broad query can't load forever; narrow the JQL to see the rest.
	MaxIssues int `yaml:"max_issues"`
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
	Transition    string `yaml:"transition,omitempty"`
	AddLabels     string `yaml:"add_labels,omitempty"`
	AddComment    string `yaml:"add_comment,omitempty"`
	Assign        string `yaml:"assign,omitempty"`
	OpenBrowser   string `yaml:"open_browser,omitempty"`
	FieldDiscover string `yaml:"field_discovery,omitempty"`
	ForceRefresh  string `yaml:"force_refresh,omitempty"`
	Help          string `yaml:"help,omitempty"`
	Sort          string `yaml:"sort,omitempty"`     // list-view sort picker (list view only; 's' also defaults transition in detail view)
	Copy          string `yaml:"copy,omitempty"`     // pick fields/URL to copy to the clipboard
	EditJQL       string `yaml:"edit_jql,omitempty"` // edit the tab's JQL; on the Search tab, type a query
}

type Config struct {
	Backend     Backend     `yaml:"backend"`
	Tabs        []Tab       `yaml:"tabs"`
	List        List        `yaml:"list"`
	Detail      Detail      `yaml:"detail"`
	Keybindings Keybindings `yaml:"keybindings,omitempty"`
}

// Save writes cfg back to path in YAML format. Keybindings are written only
// where they differ from the defaults, so a later change of default reaches
// configs the app has saved.
func Save(path string, cfg Config) error {
	cfg.Keybindings = cfg.Keybindings.withoutDefaults()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// DefaultPath returns the default config file path,
// ~/.config/jira-tabbed-tui/config.yaml. Older releases used
// ~/.config/jira-tui/config.yaml; that file is still used when it exists and
// the new one does not, so existing setups keep working.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	path := filepath.Join(home, ".config", "jira-tabbed-tui", "config.yaml")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	legacy := filepath.Join(home, ".config", "jira-tui", "config.yaml")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return path
}

// defaultConfigYAML is written to disk when no config file exists.
const defaultConfigYAML = `backend:
  cli: jira
  # url: https://myorg.atlassian.net   # base URL for "open in browser"; omit to use 'jira open KEY'
  # extra_args: []

tabs:
  - name: Assigned
    jql: assignee = currentUser()
    # sort: priority   # field to sort by (key, priority, status, assignee, duedate, …)
    # sort_asc: false  # true = ascending, false = descending (default)
    # columns:         # overrides global list.columns for this tab only
    #   - field: key
    #     label: ID
    #   - field: summary
    #   - field: priority
  - name: In Progress
    jql: assignee = currentUser() AND status = "In Progress"

list:
  # Available fields: key, type, summary, priority, status, assignee, duedate, created, updated
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

	cfg.Keybindings.migrateRetiredDefaults()
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
	if c.List.MaxIssues <= 0 {
		c.List.MaxIssues = 1000
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
		kb.Transition = "m"
	}
	if kb.AddLabels == "" {
		kb.AddLabels = "L" // l moves right, like h/j/k
	}
	if kb.AddComment == "" {
		kb.AddComment = "c"
	}
	if kb.Assign == "" {
		kb.Assign = "a"
	}
	if kb.OpenBrowser == "" {
		kb.OpenBrowser = "o"
	}
	if kb.FieldDiscover == "" {
		kb.FieldDiscover = "ctrl+f"
	}
	if kb.ForceRefresh == "" {
		kb.ForceRefresh = "r"
	}
	if kb.Help == "" {
		kb.Help = "?"
	}
	if kb.Sort == "" {
		kb.Sort = "s"
	}
	if kb.Copy == "" {
		kb.Copy = "Y"
	}
	if kb.EditJQL == "" {
		kb.EditJQL = "e"
	}
}

// retiredDefaults maps keybindings (by YAML name) from an old default key to
// its current one. Releases before Save skipped defaults wrote every key into
// the config, so a value equal to an old default most likely came from there
// rather than from the user, and now clashes or misleads:
//
//	transition s → m        (s is the sort picker)
//	add_labels l → L        (l moves right, like vim)
//	field_discovery F → ctrl+f  (matches ctrl+e, which edits a field)
var retiredDefaults = map[string][2]string{
	"transition":      {"s", "m"},
	"add_labels":      {"l", "L"},
	"field_discovery": {"F", "ctrl+f"},
}

// keyFields returns kb's keybindings by YAML name.
func (kb *Keybindings) keyFields() map[string]*string {
	fields := map[string]*string{}
	v := reflect.ValueOf(kb).Elem()
	for i := 0; i < v.NumField(); i++ {
		name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("yaml"), ",")
		fields[name] = v.Field(i).Addr().Interface().(*string)
	}
	return fields
}

// migrateRetiredDefaults moves keys still set to a retired default to the
// current one.
func (kb *Keybindings) migrateRetiredDefaults() {
	for name, key := range kb.keyFields() {
		if r, ok := retiredDefaults[name]; ok && *key == r[0] {
			*key = r[1]
		}
	}
}

// withoutDefaults blanks every keybinding equal to its default.
func (kb Keybindings) withoutDefaults() Keybindings {
	var def Config
	def.applyDefaults()
	defaults := def.Keybindings.keyFields()
	for name, key := range kb.keyFields() {
		if *key == *defaults[name] {
			*key = ""
		}
	}
	return kb
}

func (c *Config) validate() error {
	for i, tab := range c.Tabs {
		if tab.JQL == "" {
			return fmt.Errorf("tab %d (%q) is missing a jql value", i+1, tab.Name)
		}
	}
	return nil
}
