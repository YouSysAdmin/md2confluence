package converter

import (
	"regexp"
	"strings"
	"testing"
)

// reMacroID strips the random ac:macro-id from code-macro output so equality
// checks against expected fixtures are stable.
var reMacroID = regexp.MustCompile(`ac:macro-id="[^"]*"`)

func render(t *testing.T, md string) string {
	t.Helper()
	res, err := ToStorage(md, Options{})
	if err != nil {
		t.Fatalf("ToStorage: %v", err)
	}
	return reMacroID.ReplaceAllString(res.Storage, `ac:macro-id="X"`)
}

func TestEmpty(t *testing.T) {
	res, err := ToStorage("", Options{})
	if err != nil {
		t.Fatalf("ToStorage: %v", err)
	}
	if res.Storage != "" {
		t.Fatalf("want empty, got %q", res.Storage)
	}
	if len(res.LocalImages) != 0 {
		t.Fatalf("want no images, got %d", len(res.LocalImages))
	}
}

func TestHeadingsAndEmphasis(t *testing.T) {
	got := render(t, "# Title\n\n## Subtitle\n\n**bold** and *italic* and ~~strike~~ and `code`.\n")
	wants := []string{
		"<h1>Title</h1>",
		"<h2>Subtitle</h2>",
		"<strong>bold</strong>",
		"<em>italic</em>",
		"<s>strike</s>",
		"<code>code</code>",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

// Soft line breaks must render as a single space, not a literal '\n'. Confluence
// re-serializes whitespace inside text content; emitting '\n' caused every
// re-sync of a wrapped paragraph to look diffed against the server copy.
func TestSoftLineBreakIsSpace(t *testing.T) {
	got := render(t, "first line\nsecond line\n")
	if strings.Contains(got, "first line\nsecond") {
		t.Errorf("soft break still emitted as newline:\n%s", got)
	}
	if !strings.Contains(got, "first line second line") {
		t.Errorf("soft break should join lines with a space:\n%s", got)
	}
}

func TestHardLineBreakIsBR(t *testing.T) {
	got := render(t, "line one  \nline two\n") // two trailing spaces = hard break
	if !strings.Contains(got, "<br/>") {
		t.Errorf("hard break missing <br/>:\n%s", got)
	}
}

func TestFencedCodeBlock(t *testing.T) {
	got := render(t, "```sh\necho hi\n```\n")
	wants := []string{
		`<ac:structured-macro ac:name="code"`,
		`ac:schema-version="1"`,
		`ac:macro-id="X"`,
		`<ac:parameter ac:name="language">bash</ac:parameter>`,
		`<ac:plain-text-body><![CDATA[echo hi]]></ac:plain-text-body>`,
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestRemoteImage(t *testing.T) {
	got := render(t, "![alt](https://example.com/x.png)\n")
	wants := []string{
		`<ac:image`,
		`ac:alt="alt"`,
		`ac:width="760"`,
		`<ri:url ri:value="https://example.com/x.png"/>`,
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestLocalImageRecorded(t *testing.T) {
	res, err := ToStorage("![diagram](./diagram.png)\n", Options{BaseDir: "/abs/dir"})
	if err != nil {
		t.Fatalf("ToStorage: %v", err)
	}
	if !strings.Contains(res.Storage, `<ri:attachment ri:filename="diagram.png"/>`) {
		t.Errorf("local image attachment ref missing:\n%s", res.Storage)
	}
	if len(res.LocalImages) != 1 {
		t.Fatalf("want 1 local image, got %d", len(res.LocalImages))
	}
	if res.LocalImages[0].Filename != "diagram.png" {
		t.Errorf("filename = %q, want diagram.png", res.LocalImages[0].Filename)
	}
	if res.LocalImages[0].AbsolutePath != "/abs/dir/diagram.png" {
		t.Errorf("abs path = %q, want /abs/dir/diagram.png", res.LocalImages[0].AbsolutePath)
	}
}

func TestImageSizeOverride(t *testing.T) {
	got := render(t, "![alt|width=400,height=300](https://example.com/x.png)\n")
	if !strings.Contains(got, `ac:width="400"`) {
		t.Errorf("width override missing:\n%s", got)
	}
	if !strings.Contains(got, `ac:height="300"`) {
		t.Errorf("height override missing:\n%s", got)
	}
}

func TestImageDeduplicatesByFilename(t *testing.T) {
	res, err := ToStorage("![a](./img.png)\n\n![b](./img.png)\n", Options{BaseDir: "/d"})
	if err != nil {
		t.Fatalf("ToStorage: %v", err)
	}
	if len(res.LocalImages) != 1 {
		t.Errorf("want 1 image after dedup, got %d", len(res.LocalImages))
	}
}

func TestLink(t *testing.T) {
	got := render(t, "[click](https://example.com)\n")
	if !strings.Contains(got, `<a href="https://example.com">click</a>`) {
		t.Errorf("link missing:\n%s", got)
	}
}

func TestAutolink(t *testing.T) {
	got := render(t, "<https://example.com>\n")
	if !strings.Contains(got, `<a href="https://example.com">https://example.com</a>`) {
		t.Errorf("autolink missing:\n%s", got)
	}
}

func TestList(t *testing.T) {
	got := render(t, "- one\n- two\n- three\n")
	wants := []string{"<ul>", "<li>one</li>", "<li>two</li>", "<li>three</li>", "</ul>"}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestTaskList(t *testing.T) {
	got := render(t, "- [x] done\n- [ ] todo\n")
	if !strings.Contains(got, "[x] done") {
		t.Errorf("checked task missing:\n%s", got)
	}
	if !strings.Contains(got, "[ ] todo") {
		t.Errorf("unchecked task missing:\n%s", got)
	}
}

func TestTable(t *testing.T) {
	got := render(t, "| h1 | h2 |\n|----|----|\n| a  | b  |\n")
	wants := []string{
		"<table><tbody>",
		"<th>h1</th>", "<th>h2</th>",
		"<td>a</td>", "<td>b</td>",
		"</tbody></table>",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestBlockquote(t *testing.T) {
	got := render(t, "> quoted\n")
	if !strings.Contains(got, "<blockquote>") || !strings.Contains(got, "quoted") {
		t.Errorf("blockquote missing:\n%s", got)
	}
}

func TestThematicBreak(t *testing.T) {
	got := render(t, "before\n\n---\n\nafter\n")
	if !strings.Contains(got, "<hr/>") {
		t.Errorf("hr missing:\n%s", got)
	}
}

func TestEscapesXMLInText(t *testing.T) {
	got := render(t, "5 < 6 & 7 > 4\n")
	if !strings.Contains(got, "5 &lt; 6 &amp; 7 &gt; 4") {
		t.Errorf("text not escaped:\n%s", got)
	}
}

func TestCodeLangAlias(t *testing.T) {
	got := render(t, "```py\nprint(1)\n```\n")
	if !strings.Contains(got, `<ac:parameter ac:name="language">python</ac:parameter>`) {
		t.Errorf("py → python alias not applied:\n%s", got)
	}
}

func TestDefaultImageWidthOverride(t *testing.T) {
	res, err := ToStorage("![a](https://x/y.png)\n", Options{DefaultImageWidth: 400})
	if err != nil {
		t.Fatalf("ToStorage: %v", err)
	}
	if !strings.Contains(res.Storage, `ac:width="400"`) {
		t.Errorf("DefaultImageWidth not honored:\n%s", res.Storage)
	}
}

func TestParseImageAttrs(t *testing.T) {
	cases := []struct {
		raw, alt, w, h string
	}{
		{"diagram", "diagram", "", ""},
		{"diagram|width=400", "diagram", "400", ""},
		{"diagram|w=400,h=300", "diagram", "400", "300"},
		{"|width=200", "", "200", ""},
		{"alt with space|height=50", "alt with space", "", "50"},
	}
	for _, c := range cases {
		alt, w, h := parseImageAttrs(c.raw)
		if alt != c.alt || w != c.w || h != c.h {
			t.Errorf("parseImageAttrs(%q) = (%q,%q,%q), want (%q,%q,%q)",
				c.raw, alt, w, h, c.alt, c.w, c.h)
		}
	}
}

func TestMapCodeLang(t *testing.T) {
	cases := map[string]string{
		"sh":    "bash",
		"shell": "bash",
		"js":    "javascript",
		"py":    "python",
		"yml":   "yaml",
		"go":    "go",
		"":      "",
		"FOO":   "foo", // unknown passes through, lowercased
	}
	for in, want := range cases {
		if got := mapCodeLang(in); got != want {
			t.Errorf("mapCodeLang(%q) = %q, want %q", in, got, want)
		}
	}
}
