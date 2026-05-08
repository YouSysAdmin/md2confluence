package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yousysadmin/md2confluence/internal/config"
	"github.com/yousysadmin/md2confluence/internal/confluence"
	"github.com/yousysadmin/md2confluence/internal/scanner"
)

func newListCmd(f *flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pages managed by this tool (all configured spaces, or one with --space)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(f)
		},
	}
	cmd.Flags().StringVarP(&f.space, "space", "s", "", "Confluence space key to list (default: all configured spaces)")
	return cmd
}

func runList(f *flags) error {
	cfg, err := config.Load(f.cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	targets := cfg.Spaces
	if f.space != "" {
		sp := findSpaceConfig(cfg.Spaces, f.space)
		if sp == nil {
			return fmt.Errorf("space key %q not found in config (available: %s)", f.space, configuredKeys(cfg.Spaces))
		}
		targets = []config.SpaceConfig{*sp}
	}

	client := confluence.NewClient(cfg.BaseURL, cfg.Email, cfg.APIToken)

	for _, sp := range targets {
		spaceID, err := client.LookupSpaceID(sp.Key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Space %q: %v\n", sp.Key, err)
			continue
		}
		root, err := scanner.BuildTree(sp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Space %q: build tree: %v\n", sp.Key, err)
			continue
		}
		fmt.Printf("Pages in space %s:\n\n", sp.Key)
		listTree(client, spaceID, root, 0)
		fmt.Println()
	}
	return nil
}

func listTree(client *confluence.Client, spaceID string, n *scanner.Node, depth int) {
	indent := strings.Repeat("  ", depth)
	existing, err := client.SearchPage(spaceID, n.Title)
	status := "new"
	switch {
	case err != nil:
		status = fmt.Sprintf("error: %v", err)
	case existing != nil:
		status = fmt.Sprintf("id=%s v%d", existing.ID, existing.Version.Number)
	}
	fmt.Printf("%s- %-30s %s\n", indent, n.Title, status)
	for _, child := range n.Children {
		listTree(client, spaceID, child, depth+1)
	}
}
