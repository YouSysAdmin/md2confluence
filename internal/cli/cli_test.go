package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/yousysadmin/md2confluence/internal/config"
	"github.com/yousysadmin/md2confluence/internal/confluence"
)

// fakeClient is an in-memory Confluence stand-in keyed by page title.
type fakeClient struct {
	mu       sync.Mutex
	nextID   int
	pages    map[string]*confluence.Page // title -> page
	calls    []string
	loopOnce bool // make the first reparenting UpdatePage fail with a loop error
	failAll  bool // every call errors (dry-run must never reach here)
}

func newFakeClient() *fakeClient {
	return &fakeClient{nextID: 100, pages: map[string]*confluence.Page{}}
}

func (f *fakeClient) record(s string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
	if f.failAll {
		return errors.New("unexpected API call: " + s)
	}
	return nil
}

func (f *fakeClient) LookupSpaceID(_ context.Context, key string) (string, error) {
	if err := f.record("lookup " + key); err != nil {
		return "", err
	}
	return "space-" + key, nil
}

func (f *fakeClient) SearchPage(_ context.Context, _, title string) (*confluence.Page, error) {
	if err := f.record("search " + title); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pages[title]
	if !ok {
		return nil, nil
	}
	cp := *p
	return &cp, nil
}

func (f *fakeClient) CreatePage(_ context.Context, req *confluence.CreateRequest) (*confluence.Page, error) {
	if err := f.record("create " + req.Title); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	p := &confluence.Page{
		ID: strconv.Itoa(f.nextID), Title: req.Title, SpaceID: req.SpaceID, ParentID: req.ParentID,
		Version: confluence.Version{Number: 1},
		Body:    &confluence.BodyRead{Storage: &confluence.BodyStorageRead{Value: req.Body.Value, Representation: "storage"}},
	}
	f.pages[req.Title] = p
	return p, nil
}

func (f *fakeClient) UpdatePage(_ context.Context, id string, req *confluence.UpdateRequest) (*confluence.Page, error) {
	if err := f.record(fmt.Sprintf("update %s parent=%s", req.Title, req.ParentID)); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.pages[req.Title]
	if p == nil || p.ID != id {
		return nil, fmt.Errorf("update: page %s not found", id)
	}
	if f.loopOnce && req.ParentID != p.ParentID {
		f.loopOnce = false
		return nil, errors.New("PUT /api/v2/pages/1: status 400: Cannot update page: parent-child loop")
	}
	p.ParentID = req.ParentID
	p.Version.Number = req.Version.Number
	p.Body.Storage.Value = req.Body.Value
	return p, nil
}

func (f *fakeClient) UploadAttachment(_ context.Context, pageID, filePath string) error {
	return f.record("upload " + pageID + " " + filepath.Base(filePath))
}

// writeDocs creates a docs tree: root/{root.md, guide.md, sub/sub.md} and returns its path.
func writeDocs(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "docs")
	mustWrite(t, filepath.Join(dir, "docs.md"), "# Home\n\nintro\n")
	mustWrite(t, filepath.Join(dir, "guide.md"), "# Guide\n\nsome **text**\n")
	mustWrite(t, filepath.Join(dir, "sub", "sub.md"), "# Sub\n\nnested\n")
	return dir
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeConfig writes a config file mapping space PROJ to docsDir.
func writeConfig(t *testing.T, docsDir string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	mustWrite(t, path, fmt.Sprintf(`email: u@example.com
apiToken: tok
baseUrl: https://x.atlassian.net/wiki
spaces:
  - key: PROJ
    path: %q
    parentTitle: Home
    autoCreateParent: true
`, docsDir))
	return path
}

// run executes the CLI with args against the fake and returns stdout, stderr, err.
func run(t *testing.T, fc *fakeClient, args ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd(BuildInfo{Version: "test"}, func(*config.Config) pageClient { return fc })
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, ExitOK},
		{errors.New("boom"), ExitError},
		{&usageError{errors.New("bad")}, ExitUsage},
		{fmt.Errorf("wrap: %w", &usageError{errors.New("bad")}), ExitUsage},
		{errors.New(`unknown command "x" for "md2confluence"`), ExitUsage},
		{errors.New(`required flag(s) "space" not set`), ExitUsage},
		{fmt.Errorf("search: %w", context.Canceled), ExitInterrupted},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	cfg := writeConfig(t, writeDocs(t))
	for _, args := range [][]string{
		{"sync", "--bogus"},
		{"upload"},
		{"upload", "a.md"}, // missing --space
		{"nosuch"},
		{"list", "-o", "xml", "--config", cfg},
		{"list", "--space", "NOPE", "--config", cfg},
		{"sync", "extra", "--config", cfg},
	} {
		_, _, err := run(t, newFakeClient(), args...)
		if ExitCode(err) != ExitUsage {
			t.Errorf("%v: got err %v (exit %d), want usage error", args, err, ExitCode(err))
		}
	}
}

func TestUploadDryRunDoesNotCallAPI(t *testing.T) {
	docs := writeDocs(t)
	fc := newFakeClient()
	fc.failAll = true
	out, _, err := run(t, fc, "upload", filepath.Join(docs, "guide.md"), "--space", "PROJ", "--dry-run", "--config", writeConfig(t, docs))
	if err != nil {
		t.Fatalf("upload --dry-run: %v", err)
	}
	if len(fc.calls) != 0 {
		t.Errorf("dry-run made API calls: %v", fc.calls)
	}
	if !strings.Contains(out, "[DRY-RUN] Space PROJ:") || !strings.Contains(out, "- Guide") {
		t.Errorf("unexpected dry-run output:\n%s", out)
	}
}

func TestSyncDryRunJSON(t *testing.T) {
	docs := writeDocs(t)
	fc := newFakeClient()
	fc.failAll = true
	out, _, err := run(t, fc, "sync", "--dry-run", "-o", "json", "--config", writeConfig(t, docs))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		DryRun bool        `json:"dryRun"`
		Spaces []spaceTree `json:"spaces"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if !got.DryRun || len(got.Spaces) != 1 || got.Spaces[0].Tree.Title != "Home" || len(got.Spaces[0].Tree.Children) != 2 {
		t.Errorf("unexpected tree: %s", out)
	}
}

func TestSyncCreatesThenUnchanged(t *testing.T) {
	docs := writeDocs(t)
	cfg := writeConfig(t, docs)
	fc := newFakeClient()

	out, _, err := run(t, fc, "sync", "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var rep syncReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if rep.Stats.Created != 3 || rep.Stats.Updated != 0 || rep.Stats.Skipped != 0 {
		t.Errorf("first run stats = %+v, want 3 created", rep.Stats)
	}
	home, guide, sub := fc.pages["Home"], fc.pages["Guide"], fc.pages["Sub"]
	if home == nil || guide == nil || sub == nil {
		t.Fatalf("pages missing: %v", fc.calls)
	}
	if guide.ParentID != home.ID || sub.ParentID != home.ID {
		t.Errorf("children not parented to Home: guide=%s sub=%s home=%s", guide.ParentID, sub.ParentID, home.ID)
	}

	// Second run: nothing changed on disk, so every page is unchanged.
	out, _, err = run(t, fc, "sync", "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Stats.Unchanged != 3 || rep.Stats.Updated != 0 || rep.Stats.Created != 0 {
		t.Errorf("second run stats = %+v, want 3 unchanged", rep.Stats)
	}

	// Edit one file: exactly one update, version bumped.
	mustWrite(t, filepath.Join(docs, "guide.md"), "# Guide\n\nchanged\n")
	out, _, err = run(t, fc, "sync", "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Updated: PROJ/Guide") || !strings.Contains(out, "Done. Created: 0, Updated: 1, Unchanged: 2") {
		t.Errorf("unexpected text output:\n%s", out)
	}
	if fc.pages["Guide"].Version.Number != 2 {
		t.Errorf("Guide version = %d, want 2", fc.pages["Guide"].Version.Number)
	}
}

func TestSyncSelfParentGuardAndLoopRetry(t *testing.T) {
	docs := writeDocs(t)
	cfg := writeConfig(t, docs)
	fc := newFakeClient()
	// Pre-existing Home at root and Guide parented elsewhere with stale content.
	fc.pages["Home"] = &confluence.Page{ID: "1", Title: "Home", ParentID: "", Version: confluence.Version{Number: 1},
		Body: &confluence.BodyRead{Storage: &confluence.BodyStorageRead{Value: "<p>old</p>"}}}
	fc.pages["Guide"] = &confluence.Page{ID: "2", Title: "Guide", ParentID: "9", Version: confluence.Version{Number: 4},
		Body: &confluence.BodyRead{Storage: &confluence.BodyStorageRead{Value: "<p>old</p>"}}}
	fc.loopOnce = true

	_, errOut, err := run(t, fc, "sync", "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Home is the tree root with no parent to enforce: the update must preserve
	// its existing (empty) parent rather than be treated as an error.
	if fc.pages["Home"].Version.Number != 2 {
		t.Errorf("Home not updated: %+v", fc.pages["Home"])
	}
	// Guide's reparent to Home hit a loop once; the retry keeps parent 9.
	if !strings.Contains(errOut, "would create a loop") {
		t.Errorf("expected loop warning, got stderr:\n%s", errOut)
	}
	if fc.pages["Guide"].ParentID != "9" || fc.pages["Guide"].Version.Number != 5 {
		t.Errorf("Guide after loop retry = %+v", fc.pages["Guide"])
	}
}

func TestSyncSkipsOnPerPageErrorButStopsOnCancel(t *testing.T) {
	docs := writeDocs(t)
	cfg := writeConfig(t, docs)
	fc := newFakeClient()

	root := newRootCmd(BuildInfo{}, func(*config.Config) pageClient { return fc })
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "--config", cfg})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := root.ExecuteContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled sync returned %v, want context.Canceled", err)
	}
	if ExitCode(err) != ExitInterrupted {
		t.Errorf("exit code %d, want %d", ExitCode(err), ExitInterrupted)
	}
}

func TestListJSON(t *testing.T) {
	docs := writeDocs(t)
	cfg := writeConfig(t, docs)
	fc := newFakeClient()
	fc.pages["Home"] = &confluence.Page{ID: "1", Title: "Home", Version: confluence.Version{Number: 7}}

	out, _, err := run(t, fc, "list", "--space", "PROJ", "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Spaces []spaceTree `json:"spaces"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got.Spaces) != 1 {
		t.Fatalf("spaces = %d", len(got.Spaces))
	}
	tree := got.Spaces[0].Tree
	if tree.Status != "exists" || tree.ID != "1" || tree.Version != 7 {
		t.Errorf("root = %+v", tree)
	}
	if len(tree.Children) != 2 || tree.Children[0].Status != "new" {
		t.Errorf("children = %+v", tree.Children)
	}

	out, _, err = run(t, fc, "list", "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "id=1 v7") || !strings.Contains(out, "new") {
		t.Errorf("text list output:\n%s", out)
	}
}

func TestSpaceCompletion(t *testing.T) {
	cfg := writeConfig(t, writeDocs(t))
	out, _, err := run(t, newFakeClient(), "__complete", "list", "--config", cfg, "--space", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "PROJ\n") {
		t.Errorf("completion output:\n%s", out)
	}
}
