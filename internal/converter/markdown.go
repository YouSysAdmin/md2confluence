package converter

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/yousysadmin/md2confluence/internal/scanner"
)

// Options tunes how a markdown document is rendered to storage format.
type Options struct {
	// BaseDir is the directory of the source markdown file; relative image
	// paths are resolved against it.
	BaseDir string
	// DefaultImageWidth is the pixel value emitted as ac:width when an
	// image lacks an explicit |width=… hint in its alt text. Zero falls
	// back to DefaultImageWidthFallback.
	DefaultImageWidth int
}

// DefaultImageWidthFallback is the pixel width applied when Options.DefaultImageWidth is 0.
const DefaultImageWidthFallback = 760

// Result is the output of converting a markdown document.
type Result struct {
	// Storage holds the Confluence storage format XML.
	Storage string
	// LocalImages lists images referenced via relative paths that should be
	// uploaded as page attachments.
	LocalImages []LocalImage
}

// LocalImage describes one markdown image that resolves to a local file.
type LocalImage struct {
	// AbsolutePath is the resolved filesystem path to upload.
	AbsolutePath string
	// Filename is the basename referenced from the storage XML.
	Filename string
}

// ToStorage converts markdown content to Confluence storage format.
func ToStorage(md string, opts Options) (*Result, error) {
	if strings.TrimSpace(md) == "" {
		return &Result{}, nil
	}

	defaultWidth := opts.DefaultImageWidth
	if defaultWidth == 0 {
		defaultWidth = DefaultImageWidthFallback
	}

	source := []byte(md)
	gm := goldmark.New(goldmark.WithExtensions(extension.GFM))
	root := gm.Parser().Parse(text.NewReader(source))

	r := &confluenceRenderer{
		baseDir:      opts.BaseDir,
		defaultWidth: defaultWidth,
		seen:         map[string]bool{},
	}
	var buf bytes.Buffer
	if err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		return r.visit(&buf, source, n, entering)
	}); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	if r.warnedRawHTML {
		fmt.Fprintln(os.Stderr, "warning: source contained raw HTML - Confluence storage format accepts only a small subset of HTML; if the page fails with HTTP 400 \"unsupported extensions\", remove the inline HTML.")
	}

	return &Result{Storage: buf.String(), LocalImages: r.images}, nil
}

// confluenceRenderer walks a goldmark AST and writes Confluence storage format.
type confluenceRenderer struct {
	baseDir      string
	defaultWidth int
	images       []LocalImage
	seen         map[string]bool
	// linkInternalStack tracks per-link whether the open emitted an <ac:link>
	// (true) or a plain <a> (false). goldmark's ast.Walk always invokes the
	// leaving call even when we return WalkSkipChildren, so the close branch
	// needs to know which open tag to match - emitting </a> after an
	// <ac:link> would produce malformed storage XML that Confluence rejects
	// with HTTP 400 "unsupported extensions".
	linkInternalStack []bool
	// warnedRawHTML is set when the document contained inline HTML or an
	// HTML block. Confluence storage format only accepts a small subset of
	// HTML, so unknown tags can fail with HTTP 400 ("unsupported extensions").
	// We pass the content through unchanged but emit a single warning so the
	// caller can spot the likely cause if Confluence rejects the page.
	warnedRawHTML bool
}

func (r *confluenceRenderer) visit(w *bytes.Buffer, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	switch n := n.(type) {
	case *ast.Document:
		return ast.WalkContinue, nil

	case *ast.Heading:
		if entering {
			fmt.Fprintf(w, "<h%d>", n.Level)
		} else {
			fmt.Fprintf(w, "</h%d>\n", n.Level)
		}

	case *ast.Paragraph:
		if parentIsTightListItem(n) {
			return ast.WalkContinue, nil
		}
		if entering {
			w.WriteString("<p>")
		} else {
			w.WriteString("</p>\n")
		}

	case *ast.TextBlock:
		return ast.WalkContinue, nil

	case *ast.Text:
		if entering {
			w.WriteString(escapeXML(string(n.Segment.Value(source))))
			// Soft line breaks become a single space rather than a literal '\n'.
			// Confluence re-serializes whitespace inside text content, so emitting
			// '\n' would cause every re-sync of a wrapped paragraph to look diffed.
			if n.HardLineBreak() {
				w.WriteString("<br/>\n")
			} else if n.SoftLineBreak() {
				w.WriteString(" ")
			}
		}

	case *ast.Emphasis:
		tag := "em"
		if n.Level == 2 {
			tag = "strong"
		}
		if entering {
			w.WriteString("<" + tag + ">")
		} else {
			w.WriteString("</" + tag + ">")
		}

	case *ast.CodeSpan:
		if entering {
			w.WriteString("<code>")
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				if t, ok := c.(*ast.Text); ok {
					w.WriteString(escapeXML(string(t.Segment.Value(source))))
				}
			}
			w.WriteString("</code>")
			return ast.WalkSkipChildren, nil
		}

	case *ast.Link:
		if entering {
			dest := string(n.Destination)
			if title, anchor := r.resolveInternalLink(dest); title != "" {
				text := string(nodeText(n, source))
				if text == "" {
					text = title
				}
				w.WriteString("<ac:link")
				if anchor != "" {
					fmt.Fprintf(w, ` ac:anchor="%s"`, escapeXMLAttr(anchor))
				}
				w.WriteString(">")
				fmt.Fprintf(w, `<ri:page ri:content-title="%s"/>`, escapeXMLAttr(title))
				fmt.Fprintf(w, `<ac:plain-text-link-body><![CDATA[%s]]></ac:plain-text-link-body>`, escapeCDATA(text))
				w.WriteString("</ac:link>")
				r.linkInternalStack = append(r.linkInternalStack, true)
				return ast.WalkSkipChildren, nil
			}
			fmt.Fprintf(w, `<a href="%s">`, escapeXMLAttr(dest))
			r.linkInternalStack = append(r.linkInternalStack, false)
		} else {
			internal := r.linkInternalStack[len(r.linkInternalStack)-1]
			r.linkInternalStack = r.linkInternalStack[:len(r.linkInternalStack)-1]
			if !internal {
				w.WriteString("</a>")
			}
		}

	case *ast.AutoLink:
		if entering {
			url := string(n.URL(source))
			fmt.Fprintf(w, `<a href="%s">%s</a>`, escapeXMLAttr(url), escapeXML(url))
			return ast.WalkSkipChildren, nil
		}

	case *ast.Image:
		if entering {
			r.writeImage(w, source, n)
			return ast.WalkSkipChildren, nil
		}

	case *ast.FencedCodeBlock:
		if entering {
			lang := mapCodeLang(string(n.Language(source)))
			writeCodeMacro(w, source, n, lang)
			return ast.WalkSkipChildren, nil
		}

	case *ast.CodeBlock:
		if entering {
			writeCodeMacro(w, source, n, "")
			return ast.WalkSkipChildren, nil
		}

	case *ast.Blockquote:
		if entering {
			w.WriteString("<blockquote>")
		} else {
			w.WriteString("</blockquote>\n")
		}

	case *ast.List:
		tag := "ul"
		if n.IsOrdered() {
			tag = "ol"
		}
		if entering {
			w.WriteString("<" + tag + ">\n")
		} else {
			w.WriteString("</" + tag + ">\n")
		}

	case *ast.ListItem:
		if entering {
			w.WriteString("<li>")
		} else {
			w.WriteString("</li>\n")
		}

	case *ast.ThematicBreak:
		if entering {
			w.WriteString("<hr/>\n")
		}

	case *ast.RawHTML:
		if entering {
			for i := range n.Segments.Len() {
				seg := n.Segments.At(i)
				w.Write(seg.Value(source))
			}
			r.warnedRawHTML = true
			return ast.WalkSkipChildren, nil
		}

	case *ast.HTMLBlock:
		if entering {
			lines := n.Lines()
			for i := range lines.Len() {
				seg := lines.At(i)
				w.Write(seg.Value(source))
			}
			r.warnedRawHTML = true
			return ast.WalkSkipChildren, nil
		}

	case *extast.Strikethrough:
		if entering {
			w.WriteString("<s>")
		} else {
			w.WriteString("</s>")
		}

	case *extast.TaskCheckBox:
		if entering {
			if n.IsChecked {
				w.WriteString("[x] ")
			} else {
				w.WriteString("[ ] ")
			}
			return ast.WalkSkipChildren, nil
		}

	case *extast.Table:
		if entering {
			w.WriteString("<table><tbody>\n")
		} else {
			w.WriteString("</tbody></table>\n")
		}

	case *extast.TableHeader:
		if entering {
			w.WriteString("<tr>")
		} else {
			w.WriteString("</tr>\n")
		}

	case *extast.TableRow:
		if entering {
			w.WriteString("<tr>")
		} else {
			w.WriteString("</tr>\n")
		}

	case *extast.TableCell:
		tag := "td"
		if _, ok := n.Parent().(*extast.TableHeader); ok {
			tag = "th"
		}
		if entering {
			w.WriteString("<" + tag + ">")
		} else {
			w.WriteString("</" + tag + ">")
		}
	}

	return ast.WalkContinue, nil
}

// resolveInternalLink reports whether dest is a relative link to a local
// markdown file. When it is, returns the page title to link to (from the
// target file's H1 or its filename fallback) plus any URL fragment to pass
// through as ac:anchor. Returns empty title when dest is not an internal
// markdown ref or the target file is missing.
func (r *confluenceRenderer) resolveInternalLink(dest string) (title, anchor string) {
	pathPart := dest
	if i := strings.Index(dest, "#"); i >= 0 {
		pathPart = dest[:i]
		anchor = dest[i+1:]
	}
	if pathPart == "" {
		return "", ""
	}
	lower := strings.ToLower(pathPart)
	if isRemoteURL(pathPart) || strings.HasPrefix(lower, "mailto:") {
		return "", ""
	}
	if !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".markdown") {
		return "", ""
	}
	abs := pathPart
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(r.baseDir, pathPart)
	}
	abs = filepath.Clean(abs)
	if _, err := os.Stat(abs); err != nil {
		return "", ""
	}
	base := filepath.Base(abs)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	fallback := stem
	// {dirname}.md and index.md are directory-page content files; their
	// fallback should be the dirname so the link still resolves when the
	// target file has no H1.
	parent := filepath.Base(filepath.Dir(abs))
	if strings.EqualFold(stem, "index") || strings.EqualFold(stem, parent) {
		fallback = parent
	}
	return scanner.ResolveTitle(abs, fallback), anchor
}

// escapeCDATA splits the forbidden CDATA terminator across two sections so
// arbitrary text can be safely embedded inside ![CDATA[...]].
func escapeCDATA(s string) string {
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}

// writeImage emits the Confluence storage XML for an image node and, if the
// destination is local, records it for attachment upload.
func (r *confluenceRenderer) writeImage(w *bytes.Buffer, source []byte, n *ast.Image) {
	dest := string(n.Destination)
	rawAlt := string(nodeText(n, source))
	alt, width, height := parseImageAttrs(rawAlt)

	if width == "" {
		width = fmt.Sprintf("%d", r.defaultWidth)
	}

	w.WriteString("<ac:image")
	if alt != "" {
		fmt.Fprintf(w, ` ac:alt="%s"`, escapeXMLAttr(alt))
	}
	if width != "" {
		fmt.Fprintf(w, ` ac:width="%s"`, escapeXMLAttr(width))
	}
	if height != "" {
		fmt.Fprintf(w, ` ac:height="%s"`, escapeXMLAttr(height))
	}
	w.WriteString(">")

	if isRemoteURL(dest) {
		fmt.Fprintf(w, `<ri:url ri:value="%s"/>`, escapeXMLAttr(dest))
	} else {
		abs := dest
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(r.baseDir, dest)
		}
		abs = filepath.Clean(abs)
		filename := filepath.Base(abs)
		if !r.seen[filename] {
			r.seen[filename] = true
			r.images = append(r.images, LocalImage{AbsolutePath: abs, Filename: filename})
		}
		fmt.Fprintf(w, `<ri:attachment ri:filename="%s"/>`, escapeXMLAttr(filename))
	}

	w.WriteString("</ac:image>")
}

// nodeText concatenates the text content of an inline node by walking its
// descendants and collecting every *ast.Text segment. Replaces the deprecated
// ast.Node.Text(source) method.
func nodeText(n ast.Node, source []byte) []byte {
	var buf bytes.Buffer
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := c.(*ast.Text); ok {
			buf.Write(t.Segment.Value(source))
		}
		return ast.WalkContinue, nil
	})
	return buf.Bytes()
}

// parseImageAttrs extracts optional `|key=value,key=value` attributes from
// markdown image alt text. Recognized keys: width, height. Anything after the
// first `|` is treated as attributes; the text before it is the real alt.
//
// Example: `Diagram|width=400,height=300` → alt="Diagram", width="400", height="300".
func parseImageAttrs(raw string) (alt, width, height string) {
	idx := strings.Index(raw, "|")
	if idx < 0 {
		return raw, "", ""
	}
	alt = strings.TrimSpace(raw[:idx])
	for _, pair := range strings.Split(raw[idx+1:], ",") {
		eq := strings.Index(pair, "=")
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(pair[:eq]))
		val := strings.TrimSpace(pair[eq+1:])
		switch key {
		case "width", "w":
			width = val
		case "height", "h":
			height = val
		}
	}
	return alt, width, height
}

// writeCodeMacro emits a Confluence code macro with the given language hint.
// Goldmark stores block content as a sequence of line segments; we concatenate
// them, strip the trailing newline, and embed inside CDATA.
func writeCodeMacro(w *bytes.Buffer, source []byte, n ast.Node, lang string) {
	fmt.Fprintf(w, `<ac:structured-macro ac:name="code" ac:schema-version="1" ac:macro-id="%s">`, newMacroID())
	if lang != "" {
		fmt.Fprintf(w, `<ac:parameter ac:name="language">%s</ac:parameter>`, escapeXML(lang))
	}
	w.WriteString(`<ac:plain-text-body><![CDATA[`)
	lines := n.Lines()
	var sb strings.Builder
	for i := range lines.Len() {
		seg := lines.At(i)
		sb.Write(seg.Value(source))
	}
	w.WriteString(strings.TrimRight(sb.String(), "\n"))
	w.WriteString(`]]></ac:plain-text-body></ac:structured-macro>` + "\n")
}

// parentIsTightListItem reports whether n is a paragraph inside a tight-list
// list item (one whose items aren't separated by blank lines). Tight lists
// render `<li>text</li>` rather than `<li><p>text</p></li>`.
func parentIsTightListItem(n ast.Node) bool {
	p, ok := n.Parent().(*ast.ListItem)
	if !ok {
		return false
	}
	list, ok := p.Parent().(*ast.List)
	if !ok {
		return false
	}
	return list.IsTight
}

func isRemoteURL(s string) bool {
	s = strings.ToLower(s)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "data:")
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func escapeXMLAttr(s string) string {
	s = escapeXML(s)
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// newMacroID returns a random UUID-v4 string for ac:macro-id attributes.
func newMacroID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// codeLangAliases maps common markdown fence labels to values Confluence Cloud's
// code macro accepts. Unknown labels pass through unchanged.
var codeLangAliases = map[string]string{
	"sh":          "bash",
	"shell":       "bash",
	"zsh":         "bash",
	"js":          "javascript",
	"ts":          "typescript",
	"py":          "python",
	"yml":         "yaml",
	"rb":          "ruby",
	"golang":      "go",
	"c++":         "cpp",
	"c#":          "csharp",
	"objective-c": "objectivec",
}

func mapCodeLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if alias, ok := codeLangAliases[lang]; ok {
		return alias
	}
	return lang
}
