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
}

type Tab struct {
	Name string `yaml:"name"`
	JQL  string `yaml:"jql"`
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
	Detail      Detail      `yaml:"detail"`
	Keybindings Keybindings `yaml:"keybindings"`
}

// DefaultPath returns the default config file path.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "jira-tui", "config.yaml")
}

// Load reads and validates a config file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("config file not found: %s", path)
		}
		return Config{}, fmt.Errorf("reading config: %w", err)
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
