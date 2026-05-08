package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yousysadmin/md2confluence/internal/config"
	"github.com/yousysadmin/md2confluence/internal/confluence"
	"github.com/yousysadmin/md2confluence/internal/scanner"
)

func newUploadCmd(f *flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload [file]",
		Short: "Upload a single markdown file to Confluence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpload(f, args[0])
		},
	}
	cmd.Flags().StringVarP(&f.space, "space", "s", "", "Confluence space key (required)")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Show what would change without making changes")
	_ = cmd.MarkFlagRequired("space")
	return cmd
}

func runUpload(f *flags, file string) error {
	cfg, err := config.Load(f.cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	node, err := scanner.ScanSingleFile(file)
	if err != nil {
		return fmt.Errorf("scan file: %w", err)
	}

	client := confluence.NewClient(cfg.BaseURL, cfg.Email, cfg.APIToken)

	spaceID, err := client.LookupSpaceID(f.space)
	if err != nil {
		return fmt.Errorf("resolve space: %w", err)
	}

	var parentID string
	var imageWidth int
	if sp := findSpaceConfig(cfg.Spaces, f.space); sp != nil {
		imageWidth = sp.ImageWidth
		if sp.ParentTitle != "" {
			parentID = resolveParent(client, spaceID, sp.ParentTitle, sp.AutoCreateParent)
		}
	}

	stats := syncStats{}
	return syncNode(client, spaceID, f.space, parentID, node, imageWidth, &stats)
}
