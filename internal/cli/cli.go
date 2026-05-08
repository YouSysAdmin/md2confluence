// Package cli wires up the cobra commands and contains the orchestration
// logic for the md2confluence CLI. The cmd/md2confluence/main.go entry point
// only injects build metadata and calls Execute.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// BuildInfo build metadata supplied by main (populated via goreleaser ldflags).
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Shared flag bindings, scoped to a single Execute call.
type flags struct {
	cfgFile string
	dryRun  bool
	space   string
}

// Execute builds the root command tree and runs it.
func Execute(b BuildInfo) error {
	f := &flags{}

	root := &cobra.Command{
		Use:          "md2confluence",
		Short:        "Sync markdown files to Confluence pages",
		Version:      fmt.Sprintf("%s (commit %s, built %s)", b.Version, b.Commit, b.Date),
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVarP(&f.cfgFile, "config", "c", "", "config file (default $HOME/.config/md2confluence/config.yaml)")

	root.AddCommand(
		newSyncCmd(f),
		newUploadCmd(f),
		newListCmd(f),
	)

	return root.Execute()
}
