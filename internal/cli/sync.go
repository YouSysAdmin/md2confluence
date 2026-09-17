package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
	Uploaded  int `json:"attachmentsUploaded"`
}

// pageResult records what happened to one page during sync (JSON output).
type pageResult struct {
	Space       string   `json:"space"`
	Title       string   `json:"title"`
	ID          string   `json:"id,omitempty"`
	Action      string   `json:"action"` // created|updated|unchanged|skipped
	Attachments []string `json:"attachments,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// syncReport is the JSON document emitted by `sync` and `upload`.
type syncReport struct {
	Pages []pageResult `json:"pages"`
	Stats syncStats    `json:"stats"`
}

// syncer walks a page tree and reconciles it with Confluence. Progress goes
// to out (stdout) in text mode; diagnostics always go to errOut (stderr).
type syncer struct {
	client pageClient
	out    io.Writer
	errOut io.Writer
	format outputFormat
	report syncReport
}

func newSyncCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync all configured directories to Confluence",
		Long:  `Scan all configured directories and create/update Confluence pages from markdown files.`,
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSync(cmd)
		},
	}
	cmd.Flags().BoolVar(&a.dryRun, "dry-run", false, "Print the resolved page tree without contacting Confluence")
	return cmd
}

func (a *app) runSync(cmd *cobra.Command) error {
	ctx := cmd.Context()
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	format, _ := parseOutputFormat(a.output)

	cfg, client, err := a.loadConfig()
	if err != nil {
		return err
	}

	if a.dryRun {
		var trees []spaceTree
		for _, sp := range cfg.Spaces {
			root, err := scanner.BuildTree(sp)
			if err != nil {
				return fmt.Errorf("build tree for %s: %w", sp.Key, err)
			}
			if format == formatJSON {
				trees = append(trees, spaceTree{Key: sp.Key, Tree: scannedTree(root)})
				continue
			}
			fmt.Fprintf(out, "[DRY-RUN] Space %s:\n", sp.Key)
			printTree(out, root, 0)
		}
		if format == formatJSON {
			return writeJSON(out, struct {
				DryRun bool        `json:"dryRun"`
				Spaces []spaceTree `json:"spaces"`
			}{true, trees})
		}
		return nil
	}

	s := &syncer{client: client, out: out, errOut: errOut, format: format}
	for _, sp := range cfg.Spaces {
		spaceID, err := client.LookupSpaceID(ctx, sp.Key)
		if err != nil {
			return fmt.Errorf("resolve space %s: %w", sp.Key, err)
		}

		root, err := scanner.BuildTree(sp)
		if err != nil {
			return fmt.Errorf("build tree for %s: %w", sp.Key, err)
		}

		if err := s.syncNode(ctx, spaceID, sp.Key, "", root, sp.ImageWidth); err != nil {
			return err
		}
	}
	return s.finish()
}

// finish emits the summary: one line in text mode, the full report in JSON.
func (s *syncer) finish() error {
	if s.format == formatJSON {
		if s.report.Pages == nil {
			s.report.Pages = []pageResult{}
		}
		return writeJSON(s.out, s.report)
	}
	st := s.report.Stats
	fmt.Fprintf(s.out, "\nDone. Created: %d, Updated: %d, Unchanged: %d, Attachments uploaded: %d, Skipped: %d\n",
		st.Created, st.Updated, st.Unchanged, st.Uploaded, st.Skipped)
	return nil
}

// progress prints a text-mode progress line; suppressed in JSON mode so stdout
// stays a single parseable document.
func (s *syncer) progress(format string, args ...any) {
	if s.format == formatText {
		fmt.Fprintf(s.out, format, args...)
	}
}

// warn prints a diagnostic to stderr in every mode.
func (s *syncer) warn(format string, args ...any) {
	fmt.Fprintf(s.errOut, format, args...)
}

// skip records a page that could not be processed. Per-page failures don't
// abort the run, except when the context is done - then the walk stops.
func (s *syncer) skip(ctx context.Context, spaceKey, title, stage string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.warn("Error %s %s/%s: %v\n", stage, spaceKey, title, err)
	s.report.Stats.Skipped++
	s.report.Pages = append(s.report.Pages, pageResult{Space: spaceKey, Title: title, Action: "skipped", Error: err.Error()})
	return nil
}

// syncNode recursively syncs a page tree node and its descendants.
// parentID is the already-resolved Confluence page ID to use as the parent,
// or empty for the top-level node.
func (s *syncer) syncNode(ctx context.Context, spaceID, spaceKey, parentID string, n *scanner.Node, defaultImageWidth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	storage, images, err := renderNode(n, defaultImageWidth)
	if err != nil {
		return s.skip(ctx, spaceKey, n.Title, "rendering", err)
	}

	existing, err := s.client.SearchPage(ctx, spaceID, n.Title)
	if err != nil {
		return s.skip(ctx, spaceKey, n.Title, "searching", err)
	}

	effectiveParent := parentID
	if existing != nil && effectiveParent == existing.ID {
		// Title collision with our own parent - keep the page's existing parent
		// rather than orphaning it to space root by sending parentId="".
		effectiveParent = existing.ParentID
	}

	res := pageResult{Space: spaceKey, Title: n.Title}
	var pageID string
	switch {
	case existing != nil && s.pageIsUnchanged(existing, storage, effectiveParent):
		pageID = existing.ID
		res.Action = "unchanged"
		s.report.Stats.Unchanged++
		s.progress("Unchanged: %s/%s (id=%s)\n", spaceKey, n.Title, pageID)
	default:
		pageID, err = s.writePage(ctx, spaceID, effectiveParent, n.Title, storage, existing)
		if err != nil {
			return s.skip(ctx, spaceKey, n.Title, "syncing", err)
		}
		if existing != nil {
			res.Action = "updated"
			s.report.Stats.Updated++
			s.progress("Updated: %s/%s (id=%s)\n", spaceKey, n.Title, pageID)
		} else {
			res.Action = "created"
			s.report.Stats.Created++
			s.progress("Created: %s/%s (id=%s)\n", spaceKey, n.Title, pageID)
		}
	}
	res.ID = pageID

	res.Attachments, err = s.uploadAttachments(ctx, pageID, images)
	if err != nil {
		return err
	}
	s.report.Pages = append(s.report.Pages, res)

	for _, child := range n.Children {
		if err := s.syncNode(ctx, spaceID, spaceKey, pageID, child, defaultImageWidth); err != nil {
			return err
		}
	}
	return nil
}

// uploadAttachments uploads each local image to the given page and returns the
// filenames that succeeded. Per-image failures are logged and counted as
// skipped; they don't abort the sync unless the context is done.
func (s *syncer) uploadAttachments(ctx context.Context, pageID string, images []converter.LocalImage) ([]string, error) {
	var done []string
	for _, img := range images {
		if err := s.client.UploadAttachment(ctx, pageID, img.AbsolutePath); err != nil {
			if ctx.Err() != nil {
				return done, ctx.Err()
			}
			s.warn("  attachment %s: %v\n", img.Filename, err)
			continue
		}
		s.progress("  + attachment: %s\n", img.Filename)
		s.report.Stats.Uploaded++
		done = append(done, img.Filename)
	}
	return done, nil
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
func (s *syncer) writePage(ctx context.Context, spaceID, parentID, title, storage string, existing *confluence.Page) (string, error) {
	body := confluence.BodyWrite{Representation: "storage", Value: storage}
	if existing == nil {
		page, err := s.client.CreatePage(ctx, &confluence.CreateRequest{
			SpaceID:  spaceID,
			Status:   "current",
			Title:    title,
			ParentID: parentID,
			Body:     body,
		})
		if err != nil {
			return "", err
		}
		return page.ID, nil
	}

	// When the caller has no parent to enforce (root of a scanned tree),
	// preserve the page's current parent. Omitting parentId via omitempty
	// causes v2 to treat the update as a move to space root, which fails
	// with "parent-child loop" whenever the existing page already has
	// descendants pointing back at the proposed location.
	targetParent := parentID
	if targetParent == "" {
		targetParent = existing.ParentID
	}
	req := &confluence.UpdateRequest{
		ID:       existing.ID,
		Status:   "current",
		Title:    title,
		SpaceID:  spaceID,
		ParentID: targetParent,
		Body:     body,
		Version:  confluence.Version{Number: existing.Version.Number + 1},
	}
	_, err := s.client.UpdatePage(ctx, existing.ID, req)
	if err == nil {
		return existing.ID, nil
	}
	// A reparent that would create a loop (proposed parent is a descendant of
	// this page) cannot succeed; preserve the current parent so at least the
	// content update lands.
	if targetParent != existing.ParentID && isParentLoopError(err) {
		s.warn("Warning: cannot reparent %q to %s (would create a loop) - preserving current parent %s\n",
			title, targetParent, existing.ParentID)
		req.ParentID = existing.ParentID
		if _, err2 := s.client.UpdatePage(ctx, existing.ID, req); err2 != nil {
			return "", err2
		}
		return existing.ID, nil
	}
	return "", err
}

// isParentLoopError reports whether err is the Confluence 400 response that
// rejects a parent change because it would create a parent-child loop.
func isParentLoopError(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled) && strings.Contains(err.Error(), "parent-child loop")
}

// pageIsUnchanged reports whether an existing Confluence page already holds
// the rendered content and parent we'd otherwise PUT, so the update can be
// skipped. An empty effectiveParent means the caller is not asking to change
// the parent, so existing.ParentID is ignored.
func (s *syncer) pageIsUnchanged(existing *confluence.Page, newStorage, effectiveParent string) bool {
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
	if os.Getenv(config.EnvPrefix+"_DEBUG_DIFF") != "" {
		s.warn("--- normalized rendered (%s) ---\n%s\n", existing.Title, got)
		s.warn("--- normalized remote   (%s) ---\n%s\n", existing.Title, want)
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
