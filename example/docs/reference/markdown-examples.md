# Markdown Examples

A single page exercising the markdown features md2confluence
supports. Sync it and compare against the Confluence render to spot
any regressions.

## Headings

Six heading levels map to `<h1>`–`<h6>`.

### Level 3

#### Level 4

##### Level 5

###### Level 6

## Inline formatting

You can write **bold**, *italic*, ~~strikethrough~~, and `inline code`.
Combinations like ***bold italic*** also work.

## Code blocks

```go
package main

import "fmt"

func main() {
    fmt.Println("hello, confluence")
}
```

Common aliases (`sh`, `js`, `py`, `yml`, `rb`, …) are translated to
Confluence's own language labels automatically.

```bash
# shell alias → bash
for i in 1 2 3; do echo "$i"; done
```

## Lists

- first
- second
  - nested item
    - deeper nested
- third

1. step one
2. step two
3. step three

## Task lists

- [x] write the docs
- [x] wire up the CLI
- [ ] sync to Confluence
- [ ] review

## Links

Inline link: [the project repo](https://example.com/md2confluence).

Autolink: <https://example.com/>.

## Tables

| Feature | Status | Notes |
|---|---|---|
| Headings | ✓ | `h1`–`h6` |
| Tables | ✓ | GFM |
| Images | ✓ | local uploaded as attachments |
| Code blocks | ✓ | Code Snippet macro |
| Task lists | ✓ | rendered as `[x]` / `[ ]` prefix |

## Blockquote

> Keep it simple. A markdown file in git is easier to review than a
> page edited in a WYSIWYG editor.

## Horizontal rule

---

## Images

Remote images are embedded inline via `ri:url`:

![example image|width=500](https://placehold.co/500x200/png?text=md2confluence)

Local images are uploaded as attachments and referenced by filename.
Drop a PNG alongside this file and reference it with a relative path,
e.g. `![diagram](./diagram.png)`.
