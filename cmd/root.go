package cmd

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/tui"
)

var version = "dev"

var configPath string

var rootCmd = &cobra.Command{
	Use:   "jira-tui",
	Short: "A keyboard-driven Jira TUI",
	Long:  `A fast, keyboard-driven terminal UI for Jira, driven by your existing Jira CLI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if configPath == "" {
			configPath = config.DefaultPath()
		}

		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		runner := backend.NewCLIRunner(cfg.Backend.CLI, cfg.Backend.ExtraArgs)

		// Startup health check: verify the CLI binary exists.
		if _, err := exec.LookPath(cfg.Backend.CLI); err != nil {
			return fmt.Errorf("CLI binary %q not found in PATH: %w\n"+
				"Set backend.cli in your config to the correct binary name.", cfg.Backend.CLI, err)
		}

		// Cheap auth check.
		if _, err := runner.Run("me"); err != nil {
			return fmt.Errorf("CLI auth check failed (%s me): %w\n"+
				"Run `%s auth login` to authenticate.", cfg.Backend.CLI, err, cfg.Backend.CLI)
		}

		app := tui.New(cfg, configPath, runner)

		p := tea.NewProgram(app,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("running TUI: %w", err)
		}
		return nil
	},
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "path to config.yaml (default: ~/.config/jira-tui/config.yaml)")
	rootCmd.Version = version
	rootCmd.AddCommand(fieldsCmd)
}
