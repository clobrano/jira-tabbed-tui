package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/clobrano/jira-tabbed-tui/config"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	path := writeTempConfig(t, `
backend:
  cli: jira
tabs:
  - name: Assigned
    jql: assignee = currentUser()
detail:
  sidebar_width: 33
  sidebar_fields:
    - field: assignee
    - field: duedate
      label: Due
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backend.CLI != "jira" {
		t.Errorf("expected cli=jira, got %q", cfg.Backend.CLI)
	}
	if len(cfg.Tabs) != 1 {
		t.Errorf("expected 1 tab, got %d", len(cfg.Tabs))
	}
	if cfg.Tabs[0].Name != "Assigned" {
		t.Errorf("expected tab name Assigned, got %q", cfg.Tabs[0].Name)
	}
	if cfg.Detail.SidebarWidth != 33 {
		t.Errorf("expected sidebar_width=33, got %d", cfg.Detail.SidebarWidth)
	}
	if cfg.Detail.SidebarFields[1].Label != "Due" {
		t.Errorf("expected label Due, got %q", cfg.Detail.SidebarFields[1].Label)
	}
}

func TestLoadMissingFile(t *testing.T) {
	// When the file does not exist, Load should create it with defaults.
	path := filepath.Join(t.TempDir(), "subdir", "config.yaml")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("expected default config to be created, got error: %v", err)
	}
	if cfg.Backend.CLI != "jira" {
		t.Errorf("expected default cli=jira, got %q", cfg.Backend.CLI)
	}
	if len(cfg.Tabs) == 0 {
		t.Error("expected default tabs to be populated")
	}
	// File should now exist on disk.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected config file to be created at %s: %v", path, err)
	}
}

func TestLoadUnparseable(t *testing.T) {
	// Unclosed flow mapping is a genuine YAML syntax error.
	path := writeTempConfig(t, "key: {unclosed: [broken")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestLoadMissingTabJQL(t *testing.T) {
	path := writeTempConfig(t, `
backend:
  cli: jira
tabs:
  - name: Broken
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing jql, got nil")
	}
}

func TestDefaults(t *testing.T) {
	path := writeTempConfig(t, `
backend:
  cli: jira
tabs:
  - name: Work
    jql: project = PROJ
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Detail.SidebarWidth != 33 {
		t.Errorf("expected default sidebar_width=33, got %d", cfg.Detail.SidebarWidth)
	}
	if len(cfg.Detail.SidebarFields) == 0 {
		t.Error("expected default sidebar fields")
	}
	if cfg.Keybindings.Transition != "s" {
		t.Errorf("expected default transition key=s, got %q", cfg.Keybindings.Transition)
	}
	if cfg.Keybindings.Help != "?" {
		t.Errorf("expected default help key=?, got %q", cfg.Keybindings.Help)
	}
}
