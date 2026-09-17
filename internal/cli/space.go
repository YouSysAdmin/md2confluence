package cli

import (
	"context"
	"strings"

	"github.com/yousysadmin/md2confluence/internal/config"
	"github.com/yousysadmin/md2confluence/internal/confluence"
)

// configuredKeys returns a comma-separated list of configured space keys for
// error messages.
func configuredKeys(spaces []config.SpaceConfig) string {
	keys := make([]string, len(spaces))
	for i, sp := range spaces {
		keys[i] = sp.Key
	}
	return strings.Join(keys, ", ")
}

// findSpaceConfig returns the space config matching spaceKey, or nil.
func findSpaceConfig(spaces []config.SpaceConfig, spaceKey string) *config.SpaceConfig {
	for i := range spaces {
		if spaces[i].Key == spaceKey {
			return &spaces[i]
		}
	}
	return nil
}

// resolveParent looks up a parent page by title and optionally creates it if
// missing. Returns the page ID, or empty string when no parent should be set.
// Lookup/create failures are warnings (pages then land at space root); only a
// cancelled context is returned as an error.
func (s *syncer) resolveParent(ctx context.Context, spaceID, parentTitle string, autoCreate bool) (string, error) {
	if parentTitle == "" {
		return "", nil
	}
	parent, err := s.client.SearchPage(ctx, spaceID, parentTitle)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		s.warn("Warning: could not find parent page %q in space %s: %v\n", parentTitle, spaceID, err)
		return "", nil
	}
	if parent != nil {
		return parent.ID, nil
	}
	if !autoCreate {
		s.warn("Warning: parent page %q not found in space %s (pages will have no parent)\n", parentTitle, spaceID)
		return "", nil
	}
	created, err := s.client.CreatePage(ctx, &confluence.CreateRequest{
		SpaceID: spaceID,
		Status:  "current",
		Title:   parentTitle,
		Body:    confluence.BodyWrite{Representation: "storage", Value: ""},
	})
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		s.warn("Warning: could not create parent page %q in space %s: %v\n", parentTitle, spaceID, err)
		return "", nil
	}
	s.progress("Created parent page: %s (id=%s)\n", parentTitle, created.ID)
	s.report.Pages = append(s.report.Pages, pageResult{Space: spaceID, Title: parentTitle, ID: created.ID, Action: "created"})
	s.report.Stats.Created++
	return created.ID, nil
}
