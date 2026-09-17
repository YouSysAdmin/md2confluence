package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yousysadmin/md2confluence/internal/scanner"
)

func newUploadCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload <file>",
		Short: "Upload a single markdown file to Confluence",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runUpload(cmd, args[0])
		},
	}
	a.addSpaceFlag(cmd, "Confluence space key (required)")
	cmd.Flags().BoolVar(&a.dryRun, "dry-run", false, "Print the resolved page without contacting Confluence")
	_ = cmd.MarkFlagRequired("space")
	return cmd
}

func (a *app) runUpload(cmd *cobra.Command, file string) error {
	ctx := cmd.Context()
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	format, _ := parseOutputFormat(a.output)

	cfg, client, err := a.loadConfig()
	if err != nil {
		return err
	}

	node, err := scanner.ScanSingleFile(file)
	if err != nil {
		return fmt.Errorf("scan file: %w", err)
	}

	if a.dryRun {
		if format == formatJSON {
			return writeJSON(out, struct {
				DryRun bool        `json:"dryRun"`
				Spaces []spaceTree `json:"spaces"`
			}{true, []spaceTree{{Key: a.space, Tree: scannedTree(node)}}})
		}
		fmt.Fprintf(out, "[DRY-RUN] Space %s:\n", a.space)
		printTree(out, node, 0)
		return nil
	}

	spaceID, err := client.LookupSpaceID(ctx, a.space)
	if err != nil {
		return fmt.Errorf("resolve space: %w", err)
	}

	s := &syncer{client: client, out: out, errOut: errOut, format: format}

	var parentID string
	var imageWidth int
	if sp := findSpaceConfig(cfg.Spaces, a.space); sp != nil {
		imageWidth = sp.ImageWidth
		if sp.ParentTitle != "" {
			parentID, err = s.resolveParent(ctx, spaceID, sp.ParentTitle, sp.AutoCreateParent)
			if err != nil {
				return err
			}
		}
	}

	if err := s.syncNode(ctx, spaceID, a.space, parentID, node, imageWidth); err != nil {
		return err
	}
	return s.finish()
}
