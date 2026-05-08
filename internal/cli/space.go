package cli

import (
	"fmt"
	"os"
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
// missing. Returns the page ID or empty string when no parent should be set.
func resolveParent(client *confluence.Client, spaceID, parentTitle string, autoCreate bool) string {
	if parentTitle == "" {
		return ""
	}
	parent, err := client.SearchPage(spaceID, parentTitle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not find parent page '%s' in space %s: %v\n", parentTitle, spaceID, err)
		return ""
	}
	if parent != nil {
		return parent.ID
	}
	if !autoCreate {
		fmt.Fprintf(os.Stderr, "Warning: parent page '%s' not found in space %s (pages will have no parent)\n", parentTitle, spaceID)
		return ""
	}
	created, err := client.CreatePage(&confluence.CreateRequest{
		SpaceID: spaceID,
		Status:  "current",
		Title:   parentTitle,
		Body:    confluence.BodyWrite{Representation: "storage", Value: ""},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create parent page '%s' in space %s: %v\n", parentTitle, spaceID, err)
		return ""
	}
	fmt.Printf("Created parent page: %s (id=%s)\n", parentTitle, created.ID)
	return created.ID
}
