package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/config"
)

var fieldsCmd = &cobra.Command{
	Use:   "fields [ISSUE-KEY]",
	Short: "Print field name-to-ID mapping for an issue (no TUI)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		issueKey := args[0]

		if configPath == "" {
			configPath = config.DefaultPath()
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		runner := backend.NewCLIRunner(cfg.Backend.CLI, cfg.Backend.ExtraArgs)
		fields, err := backend.FetchAllFields(runner, issueKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		// Sort alphabetically by display name for readability.
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].DisplayName < fields[j].DisplayName
		})

		fmt.Printf("%-32s  %s\n", "Field Name", "Field ID")
		fmt.Printf("%-32s  %s\n", "----------", "--------")
		for _, f := range fields {
			fmt.Printf("%-32s  %s\n", f.DisplayName, f.ID)
		}
		return nil
	},
}
