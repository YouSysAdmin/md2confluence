package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yousysadmin/md2confluence/internal/config"
	"github.com/yousysadmin/md2confluence/internal/scanner"
)

func newListCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pages managed by this tool (all configured spaces, or one with --space)",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runList(cmd)
		},
	}
	a.addSpaceFlag(cmd, "Confluence space key to list (default: all configured spaces)")
	return cmd
}

func (a *app) runList(cmd *cobra.Command) error {
	ctx := cmd.Context()
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	format, _ := parseOutputFormat(a.output)

	cfg, client, err := a.loadConfig()
	if err != nil {
		return err
	}

	targets := cfg.Spaces
	if a.space != "" {
		sp := findSpaceConfig(cfg.Spaces, a.space)
		if sp == nil {
			return &usageError{fmt.Errorf("space key %q not found in config (available: %s)", a.space, configuredKeys(cfg.Spaces))}
		}
		targets = []config.SpaceConfig{*sp}
	}

	var trees []spaceTree
	for _, sp := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		spaceID, err := client.LookupSpaceID(ctx, sp.Key)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Fprintf(errOut, "Space %q: %v\n", sp.Key, err)
			continue
		}
		root, err := scanner.BuildTree(sp)
		if err != nil {
			fmt.Fprintf(errOut, "Space %q: build tree: %v\n", sp.Key, err)
			continue
		}
		tree, err := a.listTree(ctx, client, spaceID, root)
		if err != nil {
			return err
		}
		if format == formatJSON {
			trees = append(trees, spaceTree{Key: sp.Key, Tree: tree})
			continue
		}
		fmt.Fprintf(out, "Pages in space %s:\n\n", sp.Key)
		printListTree(out, tree, 0)
		fmt.Fprintln(out)
	}

	if format == formatJSON {
		if trees == nil {
			trees = []spaceTree{}
		}
		return writeJSON(out, struct {
			Spaces []spaceTree `json:"spaces"`
		}{trees})
	}
	return nil
}

// listTree looks up every node of the scanned tree in Confluence and returns
// the annotated tree. Only a cancelled context aborts the walk; per-page
// lookup failures are recorded on the node.
func (a *app) listTree(ctx context.Context, client pageClient, spaceID string, n *scanner.Node) (*treeNode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t := &treeNode{Title: n.Title, Source: n.ContentPath, Status: "new"}
	existing, err := client.SearchPage(ctx, spaceID, n.Title)
	switch {
	case err != nil:
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		t.Status = "error"
		t.Error = err.Error()
	case existing != nil:
		t.Status = "exists"
		t.ID = existing.ID
		t.Version = existing.Version.Number
	}
	for _, child := range n.Children {
		ct, err := a.listTree(ctx, client, spaceID, child)
		if err != nil {
			return nil, err
		}
		t.Children = append(t.Children, ct)
	}
	return t, nil
}

func printListTree(w io.Writer, t *treeNode, depth int) {
	indent := strings.Repeat("  ", depth)
	status := t.Status
	switch t.Status {
	case "exists":
		status = fmt.Sprintf("id=%s v%d", t.ID, t.Version)
	case "error":
		status = "error: " + t.Error
	}
	fmt.Fprintf(w, "%s- %-30s %s\n", indent, t.Title, status)
	for _, c := range t.Children {
		printListTree(w, c, depth+1)
	}
}
