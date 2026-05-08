package scanner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/yousysadmin/md2confluence/internal/config"
)

// Node represents one Confluence page derived from the docs tree.
// A Node may correspond to a directory (with an optional content file)
// or to a loose markdown file inside a directory.
type Node struct {
	// Title is the page title (from the content file's first H1 if present,
	// otherwise derived from the directory or file name).
	Title string
	// ContentPath is the path to the markdown file providing this page's
	// body. It may be empty for directory pages that lack a {dirname}.md or
	// index.md file - such pages are created with an empty body.
	ContentPath string
	// BaseDir is the directory relative image links should resolve against.
	// For directory pages it's the directory itself; for loose files it's
	// the enclosing directory.
	BaseDir string
	// Children are the child pages of this node.
	Children []*Node
}

// BuildTree builds a page tree for a configured space.
//
// The rules:
//   - Every directory under sp.Path becomes one page.
//   - A directory page's body comes from {dirname}.md (case-insensitive)
//     or, failing that, index.md. If neither exists the page has empty body.
//   - Every other .md / .markdown file in the directory becomes a child page
//     of that directory's page.
//   - The tree root corresponds to sp.Path itself; its title is sp.ParentTitle
//     (or, if ParentTitle is empty, the title-cased directory name).
func BuildTree(sp config.SpaceConfig) (*Node, error) {
	absPath, err := filepath.Abs(sp.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve path %s: %w", sp.Path, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", absPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("configured path is not a directory: %s", absPath)
	}

	root, err := buildDirNode(absPath)
	if err != nil {
		return nil, err
	}

	// Override the root title with the configured ParentTitle when set.
	if sp.ParentTitle != "" {
		root.Title = sp.ParentTitle
	}

	return root, nil
}

// buildDirNode recursively builds a Node for a single directory.
func buildDirNode(dir string) (*Node, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	dirName := filepath.Base(dir)
	contentPath := findContentFile(dir, entries, dirName)

	node := &Node{
		Title:       resolveTitle(contentPath, dirName),
		ContentPath: contentPath,
		BaseDir:     dir,
	}

	// Deterministic order for stable child processing.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			child, err := buildDirNode(path)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, child)
			continue
		}
		// Skip the file that became this dir's content.
		if path == contentPath {
			continue
		}
		if !isMarkdown(e.Name()) {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		node.Children = append(node.Children, &Node{
			Title:       resolveTitle(path, stem),
			ContentPath: path,
			BaseDir:     dir,
		})
	}

	return node, nil
}

// findContentFile returns the path of the file that provides the directory
// page's body: {dirname}.md first, then index.md (either case). Empty string
// if neither exists.
func findContentFile(dir string, entries []os.DirEntry, dirName string) string {
	var dirMatch, indexMatch string
	lowerDir := strings.ToLower(dirName)
	for _, e := range entries {
		if e.IsDir() || !isMarkdown(e.Name()) {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		switch stem {
		case lowerDir:
			dirMatch = filepath.Join(dir, e.Name())
		case "index":
			indexMatch = filepath.Join(dir, e.Name())
		}
	}
	if dirMatch != "" {
		return dirMatch
	}
	return indexMatch
}

// ScanSingleFile returns a single-file Node for the `upload` command.
// The caller provides the space key separately.
func ScanSingleFile(filePath string) (*Node, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("stat %s: %w", absPath, err)
	}
	stem := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	return &Node{
		Title:       resolveTitle(absPath, stem),
		ContentPath: absPath,
		BaseDir:     filepath.Dir(absPath),
	}, nil
}

// resolveTitle returns the page title for a markdown file:
// the first H1 heading encountered outside a fenced code block, or the
// title-cased fallback if no heading exists or the file is absent.
func resolveTitle(contentPath, fallback string) string {
	fallbackTitle := titleCase(fallback)
	if contentPath == "" {
		return fallbackTitle
	}
	f, err := os.Open(contentPath)
	if err != nil {
		return fallbackTitle
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Allow long lines (large embedded HTML/base64 images) - default 64 KB
	// would silently abort the scan and fall back to the title-cased filename.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	inFence := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return fallbackTitle
}

func isMarkdown(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// titleCase converts a string to Title Case, treating -, _ and whitespace
// as word separators.
func titleCase(s string) string {
	runes := []rune(s)
	nextUpper := true
	for i, r := range runes {
		if unicode.IsSpace(r) || r == '-' || r == '_' {
			nextUpper = true
		} else if nextUpper {
			runes[i] = unicode.ToUpper(r)
			nextUpper = false
		} else {
			runes[i] = unicode.ToLower(r)
		}
	}
	return string(runes)
}
