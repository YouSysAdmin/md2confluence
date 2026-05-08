package cli

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yousysadmin/md2confluence/internal/config"
	"github.com/yousysadmin/md2confluence/internal/confluence"
	"github.com/yousysadmin/md2confluence/internal/converter"
	"github.com/yousysadmin/md2confluence/internal/scanner"
)

// syncStats aggregates counts emitted from tree traversal.
type syncStats struct {
	created, updated, unchanged, skipped, uploaded int
}

func newSyncCmd(f *flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync all configured directories to Confluence",
		Long:  `Scan all configured directories and create/update Confluence pages from markdown files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(f)
		},
	}
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Show what would change without making changes")
	return cmd
}

func runSync(f *flags) error {
	cfg, err := config.Load(f.cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if f.dryRun {
		for _, sp := range cfg.Spaces {
			root, err := scanner.BuildTree(sp)
			if err != nil {
				return fmt.Errorf("build tree for %s: %w", sp.Key, err)
			}
			fmt.Printf("[DRY-RUN] Space %s:\n", sp.Key)
			printTree(root, 0)
		}
		return nil
	}

	client := confluence.NewClient(cfg.BaseURL, cfg.Email, cfg.APIToken)

	stats := syncStats{}
	for _, sp := range cfg.Spaces {
		spaceID, err := client.LookupSpaceID(sp.Key)
		if err != nil {
			return fmt.Errorf("resolve space %s: %w", sp.Key, err)
		}

		root, err := scanner.BuildTree(sp)
		if err != nil {
			return fmt.Errorf("build tree for %s: %w", sp.Key, err)
		}

		if err := syncNode(client, spaceID, sp.Key, "", root, sp.ImageWidth, &stats); err != nil {
			return err
		}
	}

	fmt.Printf("\nDone. Created: %d, Updated: %d, Unchanged: %d, Attachments uploaded: %d, Skipped: %d\n",
		stats.created, stats.updated, stats.unchanged, stats.uploaded, stats.skipped)
	return nil
}

// printTree renders a page tree indented by depth for dry-run output.
func printTree(n *scanner.Node, depth int) {
	indent := strings.Repeat("  ", depth)
	src := n.ContentPath
	if src == "" {
		src = "(empty)"
	}
	fmt.Printf("%s- %s  <- %s\n", indent, n.Title, src)
	for _, c := range n.Children {
		printTree(c, depth+1)
	}
}

// syncNode recursively syncs a page tree node and its descendants.
// parentID is the already-resolved Confluence page ID to use as the parent,
// or empty for the top-level node.
func syncNode(client *confluence.Client, spaceID, spaceKey, parentID string, n *scanner.Node, defaultImageWidth int, stats *syncStats) error {
	storage, images, err := renderNode(n, defaultImageWidth)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering %s/%s: %v\n", spaceKey, n.Title, err)
		stats.skipped++
		return nil
	}

	existing, err := client.SearchPage(spaceID, n.Title)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error searching %s/%s: %v\n", spaceKey, n.Title, err)
		stats.skipped++
		return nil
	}

	effectiveParent := parentID
	if existing != nil && effectiveParent == existing.ID {
		// Title collision with our own parent - keep the page's existing parent
		// rather than orphaning it to space root by sending parentId="".
		effectiveParent = existing.ParentID
	}

	if existing != nil && pageIsUnchanged(existing, storage, effectiveParent) {
		fmt.Printf("Unchanged: %s/%s (id=%s)\n", spaceKey, n.Title, existing.ID)
		stats.unchanged++
		uploadAttachments(client, existing.ID, images, stats)
		for _, child := range n.Children {
			if err := syncNode(client, spaceID, spaceKey, existing.ID, child, defaultImageWidth, stats); err != nil {
				return err
			}
		}
		return nil
	}

	pageID, err := writePage(client, spaceID, effectiveParent, n.Title, storage, existing)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error syncing %s/%s: %v\n", spaceKey, n.Title, err)
		stats.skipped++
		return nil
	}

	if existing != nil {
		fmt.Printf("Updated: %s/%s (id=%s)\n", spaceKey, n.Title, pageID)
		stats.updated++
	} else {
		fmt.Printf("Created: %s/%s (id=%s)\n", spaceKey, n.Title, pageID)
		stats.created++
	}

	uploadAttachments(client, pageID, images, stats)

	for _, child := range n.Children {
		if err := syncNode(client, spaceID, spaceKey, pageID, child, defaultImageWidth, stats); err != nil {
			return err
		}
	}
	return nil
}

// uploadAttachments uploads each local image to the given page. Per-image
// failures are logged and counted as skipped; they don't abort the sync.
func uploadAttachments(client *confluence.Client, pageID string, images []converter.LocalImage, stats *syncStats) {
	for _, img := range images {
		if err := client.UploadAttachment(pageID, img.AbsolutePath); err != nil {
			fmt.Fprintf(os.Stderr, "  attachment %s: %v\n", img.Filename, err)
			continue
		}
		fmt.Printf("  + attachment: %s\n", img.Filename)
		stats.uploaded++
	}
}

// renderNode reads the node's content file (if any) and converts it to storage
// format plus the list of local images to attach.
func renderNode(n *scanner.Node, defaultImageWidth int) (string, []converter.LocalImage, error) {
	if n.ContentPath == "" {
		return "", nil, nil
	}
	data, err := os.ReadFile(n.ContentPath)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", n.ContentPath, err)
	}
	res, err := converter.ToStorage(string(data), converter.Options{
		BaseDir:           n.BaseDir,
		DefaultImageWidth: defaultImageWidth,
	})
	if err != nil {
		return "", nil, err
	}
	return res.Storage, res.LocalImages, nil
}

// writePage creates or updates a page and returns its ID.
func writePage(client *confluence.Client, spaceID, parentID, title, storage string, existing *confluence.Page) (string, error) {
	body := confluence.BodyWrite{Representation: "storage", Value: storage}
	if existing != nil {
		req := &confluence.UpdateRequest{
			ID:       existing.ID,
			Status:   "current",
			Title:    title,
			SpaceID:  spaceID,
			ParentID: parentID,
			Body:     body,
			Version:  confluence.Version{Number: existing.Version.Number + 1},
		}
		if _, err := client.UpdatePage(existing.ID, req); err != nil {
			return "", err
		}
		return existing.ID, nil
	}
	req := &confluence.CreateRequest{
		SpaceID:  spaceID,
		Status:   "current",
		Title:    title,
		ParentID: parentID,
		Body:     body,
	}
	page, err := client.CreatePage(req)
	if err != nil {
		return "", err
	}
	return page.ID, nil
}

// pageIsUnchanged reports whether an existing Confluence page already holds
// the rendered content and parent we'd otherwise PUT, so the update can be
// skipped. An empty effectiveParent means the caller is not asking to change
// the parent, so existing.ParentID is ignored.
func pageIsUnchanged(existing *confluence.Page, newStorage, effectiveParent string) bool {
	if existing.Body == nil || existing.Body.Storage == nil {
		return false
	}
	if effectiveParent != "" && existing.ParentID != effectiveParent {
		return false
	}
	got := normalizeStorage(newStorage)
	want := normalizeStorage(existing.Body.Storage.Value)
	if got == want {
		return true
	}
	if os.Getenv("MD2CONFLUENCE_DEBUG_DIFF") != "" {
		fmt.Fprintf(os.Stderr, "--- normalized rendered (%s) ---\n%s\n", existing.Title, got)
		fmt.Fprintf(os.Stderr, "--- normalized remote   (%s) ---\n%s\n", existing.Title, want)
	}
	return false
}

// Storage-format normalization for equality checks between the body we render
// and the body Confluence returns. Confluence re-serializes stored content
// (whitespace between tags, space before self-close, server-side attributes
// like ri:version-at-save on attachments) and code macros carry a random
// ac:macro-id regenerated every render - all would otherwise cause false diffs.
var (
	reMacroID       = regexp.MustCompile(`\s+ac:macro-id="[^"]*"`)
	reVersionAtSave = regexp.MustCompile(`\s+ri:version-at-save="[^"]*"`)
	reSelfCloseWS   = regexp.MustCompile(`\s+/>`)
	reBetweenTags   = regexp.MustCompile(`>\s+<`)
)

func normalizeStorage(s string) string {
	s = reMacroID.ReplaceAllString(s, "")
	s = reVersionAtSave.ReplaceAllString(s, "")
	s = reSelfCloseWS.ReplaceAllString(s, "/>")
	s = reBetweenTags.ReplaceAllString(s, "><")
	return strings.TrimSpace(s)
}
