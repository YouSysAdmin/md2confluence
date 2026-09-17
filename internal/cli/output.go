package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yousysadmin/md2confluence/internal/scanner"
)

// outputFormat selects between human-readable and machine-readable output.
type outputFormat string

const (
	formatText outputFormat = "text"
	formatJSON outputFormat = "json"
)

func outputFormatNames() []string { return []string{string(formatText), string(formatJSON)} }

func parseOutputFormat(s string) (outputFormat, error) {
	switch outputFormat(strings.ToLower(s)) {
	case formatText:
		return formatText, nil
	case formatJSON:
		return formatJSON, nil
	}
	return "", fmt.Errorf("invalid --output %q (want %s)", s, strings.Join(outputFormatNames(), "|"))
}

// writeJSON emits v as indented JSON followed by a newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// treeNode is the JSON shape of a scanned page tree (dry-run and list).
type treeNode struct {
	Title    string      `json:"title"`
	Source   string      `json:"source,omitempty"`
	ID       string      `json:"id,omitempty"`
	Version  int         `json:"version,omitempty"`
	Status   string      `json:"status,omitempty"`
	Error    string      `json:"error,omitempty"`
	Children []*treeNode `json:"children,omitempty"`
}

// spaceTree pairs a space key with its page tree for JSON output.
type spaceTree struct {
	Key  string    `json:"key"`
	Tree *treeNode `json:"tree"`
}

// scannedTree converts a scanner tree into its JSON shape (title + source only).
func scannedTree(n *scanner.Node) *treeNode {
	t := &treeNode{Title: n.Title, Source: n.ContentPath}
	for _, c := range n.Children {
		t.Children = append(t.Children, scannedTree(c))
	}
	return t
}

// printTree renders a page tree indented by depth for dry-run text output.
func printTree(w io.Writer, n *scanner.Node, depth int) {
	indent := strings.Repeat("  ", depth)
	src := n.ContentPath
	if src == "" {
		src = "(empty)"
	}
	fmt.Fprintf(w, "%s- %s  <- %s\n", indent, n.Title, src)
	for _, c := range n.Children {
		printTree(w, c, depth+1)
	}
}
