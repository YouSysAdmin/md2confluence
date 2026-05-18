package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yousysadmin/md2confluence/internal/config"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestTitleCase(t *testing.T) {
	cases := map[string]string{
		"hello":         "Hello",
		"hello-world":   "Hello-World",
		"my_doc_name":   "My_Doc_Name",
		"FOO":           "Foo",
		"installation":  "Installation",
		"api reference": "Api Reference",
	}
	for in, want := range cases {
		if got := titleCase(in); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveTitleFromH1(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.md")
	writeFile(t, p, "intro line\n\n# Real Title\n\nbody\n")
	if got := ResolveTitle(p, "fallback"); got != "Real Title" {
		t.Errorf("got %q, want Real Title", got)
	}
}

func TestResolveTitleSkipsHeadingsInsideCodeFence(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.md")
	writeFile(t, p, "```\n# Not A Title\n```\n\n# Real Title\n")
	if got := ResolveTitle(p, "fallback"); got != "Real Title" {
		t.Errorf("got %q, want Real Title", got)
	}
}

func TestResolveTitleFallback(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.md")
	writeFile(t, p, "no heading here\n")
	if got := ResolveTitle(p, "my-page"); got != "My-Page" {
		t.Errorf("got %q, want My-Page", got)
	}
}

func TestResolveTitleMissingFile(t *testing.T) {
	if got := ResolveTitle("/nonexistent/x.md", "my page"); got != "My Page" {
		t.Errorf("got %q, want My Page", got)
	}
}

// {dirname}.md takes precedence over index.md as the directory page's body.
func TestBuildTreeContentFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	writeFile(t, filepath.Join(docs, "docs.md"), "# Docs Home\n")
	writeFile(t, filepath.Join(docs, "index.md"), "# Should Not Win\n")

	root, err := BuildTree(config.SpaceConfig{Path: docs})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if root.Title != "Docs Home" {
		t.Errorf("title = %q, want Docs Home", root.Title)
	}
	if filepath.Base(root.ContentPath) != "docs.md" {
		t.Errorf("content = %q, want docs.md", root.ContentPath)
	}
}

func TestBuildTreeIndexFallback(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	writeFile(t, filepath.Join(docs, "index.md"), "# Index Title\n")

	root, err := BuildTree(config.SpaceConfig{Path: docs})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if root.Title != "Index Title" {
		t.Errorf("title = %q, want Index Title", root.Title)
	}
}

func TestBuildTreeChildPagesAndSubdirs(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	writeFile(t, filepath.Join(docs, "docs.md"), "# Docs\n")
	writeFile(t, filepath.Join(docs, "intro.md"), "# Intro\n")
	writeFile(t, filepath.Join(docs, "guides", "guides.md"), "# Guides\n")
	writeFile(t, filepath.Join(docs, "guides", "deploy.md"), "# Deploy\n")

	root, err := BuildTree(config.SpaceConfig{Path: docs})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if got := len(root.Children); got != 2 {
		t.Fatalf("root children = %d, want 2", got)
	}

	// Children are sorted alphabetically by entry name (guides/, intro.md).
	guides := root.Children[0]
	intro := root.Children[1]
	if guides.Title != "Guides" {
		t.Errorf("guides title = %q, want Guides", guides.Title)
	}
	if intro.Title != "Intro" {
		t.Errorf("intro title = %q, want Intro", intro.Title)
	}
	if len(guides.Children) != 1 || guides.Children[0].Title != "Deploy" {
		t.Errorf("guides should have one child 'Deploy', got %+v", guides.Children)
	}
}

func TestBuildTreeParentTitleOverridesRoot(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	writeFile(t, filepath.Join(docs, "docs.md"), "# Should Be Overridden\n")

	root, err := BuildTree(config.SpaceConfig{Path: docs, ParentTitle: "Custom Root"})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if root.Title != "Custom Root" {
		t.Errorf("title = %q, want Custom Root", root.Title)
	}
}

func TestScanSingleFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "guide.md")
	writeFile(t, p, "# My Guide\n")

	node, err := ScanSingleFile(p)
	if err != nil {
		t.Fatalf("ScanSingleFile: %v", err)
	}
	if node.Title != "My Guide" {
		t.Errorf("title = %q, want My Guide", node.Title)
	}
	if filepath.Base(node.ContentPath) != "guide.md" {
		t.Errorf("content path = %q", node.ContentPath)
	}
	if node.BaseDir != dir {
		t.Errorf("base dir = %q, want %q", node.BaseDir, dir)
	}
}
