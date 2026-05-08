# md2confluence

Sync local markdown files to Atlassian Confluence Cloud pages.

## Features

- **Directory -> page hierarchy** - your `docs/` tree maps 1:1 to a Confluence page tree (each dir is a page; nested
  dirs
  are child pages).
- **Full CommonMark + GFM** via goldmark: headings, emphasis, strikethrough, inline/fenced code, lists (nested), task
  lists, tables, links, autolinks, blockquotes, horizontal rules, hard/soft breaks.
- **Images** - remote URLs embed directly; local images upload as page attachments automatically.
- **Confluence code blocks** - fenced code renders as Confluence's native Code Snippet macro with syntax highlighting.
- **Incremental sync** - pages whose rendered body matches what's already on Confluence are skipped, so re-runs only
  touch what changed.
- **Dry-run** - preview the page tree resolved from your directory structure without hitting Confluence.
- **Single file upload** and **page listing** commands.

## Installation

### From a release archive (recommended)

Grab the archive for your platform from the [releases page](../../releases),
extract it, and drop the binary somewhere on your `$PATH`:

```bash
tar -xzf md2confluence_<version>_Linux_x86_64.tar.gz
sudo mv md2confluence /usr/local/bin/
md2confluence --version
```

Windows builds ship as `.zip`; macOS / Linux as `.tar.gz`.

### From source

Requires Go 1.26+.

```bash
git clone <this-repo>
cd md2confluence
go build -o md2confluence ./cmd/md2confluence
```

## Try it with the bundled example

The `example/` directory contains a ready-to-run config and a small
docs tree that exercises every supported markdown feature:

```bash
md2confluence sync --config ./example/config.yaml --dry-run
```

`--dry-run` is offline, so you can preview the page tree before
supplying real credentials.

## Configuration

Config is loaded with [viper](https://github.com/spf13/viper). The default location is
`~/.config/md2confluence/config.yaml`; override with `--config /path/to/file.yaml`.

Environment variables take precedence over values in the config file and use the `MD2CONFLUENCE_` prefix:

| Env var                   | Overrides  |
|---------------------------|------------|
| `MD2CONFLUENCE_EMAIL`     | `email`    |
| `MD2CONFLUENCE_API_TOKEN` | `apiToken` |
| `MD2CONFLUENCE_BASE_URL`  | `baseUrl`  |

The `spaces` list is only read from the config file - it is not settable via env vars.

Example config file:

```yaml
email: "your-email@example.com"
apiToken: "ATATT3xFfGF0..."
baseUrl: "https://your-domain.atlassian.net/wiki"
spaces:
  - key: "DOCS"
    path: "./docs"
    parentTitle: "Documentation Home"
    autoCreateParent: true
  - key: "ENG"
    path: "./engineering"
```

### Config fields

| Field                       | Required | Description                                                                                     |
|-----------------------------|----------|-------------------------------------------------------------------------------------------------|
| `email`                     | Yes      | Your Atlassian account email                                                                    |
| `apiToken`                  | Yes      | API token from [Atlassian Account](https://id.atlassian.com/manage-profile/security/api-tokens) |
| `baseUrl`                   | Yes      | Confluence instance URL (e.g. `https://your-domain.atlassian.net/wiki`)                         |
| `spaces[].key`              | Yes      | Target space key (e.g. `DOCS`, `ENG`)                                                           |
| `spaces[].path`             | Yes      | Local directory to scan for `.md` files                                                         |
| `spaces[].parentTitle`      | No       | Parent page title under which pages are created                                                 |
| `spaces[].autoCreateParent` | No       | Create parent page if it doesn't exist (default: false)                                         |
| `spaces[].imageWidth`       | No       | Default `ac:width` (pixels) for images without an explicit size hint (default: 760)             |

### Generating an API Token

1. Go to [Atlassian Account -> Security](https://id.atlassian.com/manage-profile/security/api-tokens)
2. Click **Create API token**
3. Give it a label and copy the token - you won't see it again

## Usage

### Sync all configured directories

```bash
md2confluence sync
```

Scans every directory listed in your config, converts each `.md` file to Confluence Storage Format, and creates or
updates pages accordingly. Pages are matched by space key + title (derived from filename).

#### Dry run

See what would change without making any modifications:

```bash
md2confluence sync --dry-run
```

### Upload a single file

```bash
md2confluence upload ./docs/guide.md --space DOCS
```

### List pages in a space

```bash
md2confluence list --space DOCS
```

Shows each markdown file and whether it already exists on Confluence.

## How the directory tree maps to pages

Every directory becomes a Confluence page. The page's body comes from:

1. `{dirname}.md` inside the directory, if present, else
2. `index.md`, else
3. empty body (placeholder page).

Every other `.md` / `.markdown` file in a directory (not the content file) becomes a child page of that directory's
page.

Example:

```
docs/
├── docs.md                        -> root page body ("Documentation Home")
├── installation/
│   ├── installation.md            -> "Installation" page body
│   └── prerequisites.md           -> child page under "Installation"
├── guides/
│   ├── index.md                   -> "Guides" page body (index.md fallback)
│   └── deploy/
│       ├── deploy.md              -> "Deploy" page body (child of "Guides")
│       └── rollback.md            -> child of "Deploy"
└── api-reference/                 -> empty-body page (no matching md file)
    └── endpoints.md               -> child of "Api Reference"
```

Page titles come from the first `# Heading` in the content file (outside any code fence), falling back to the
title-cased dirname or filename.

## Markdown support

Rendered by [goldmark](https://github.com/yuin/goldmark) with GFM extensions.

| Markdown                                  | Confluence output                                                                               |
|-------------------------------------------|-------------------------------------------------------------------------------------------------|
| `# Heading` - `###### Heading`            | `<h1>` - `<h6>`                                                                                 |
| `**bold**`, `*italic*`, `~~strike~~`      | `<strong>`, `<em>`, `<s>`                                                                       |
| `` `inline` ``                            | `<code>`                                                                                        |
| ```` ```bash ``` ```` fenced block        | Confluence Code Snippet macro (`<ac:structured-macro ac:name="code">`) with syntax highlighting |
| `- item` / `1. item` / nested             | `<ul>` / `<ol>`, nested correctly                                                               |
| `- [x]` / `- [ ]` task                    | `[x]` / `[ ]` prefix in list item                                                               |
| `[text](url)`                             | `<a href="url">text</a>`                                                                        |
| `<https://…>` autolink                    | `<a href="…">…</a>`                                                                             |
| `![alt](./img.png)`                       | Uploaded as attachment; referenced via `<ac:image><ri:attachment/></ac:image>`                  |
| `![alt](https://…/img.png)`               | Inlined via `<ac:image><ri:url ri:value="…"/></ac:image>`                                       |
| `![alt\|width=400,height=300](./img.png)` | Per-image size override (`w`/`h` also accepted as short keys)                                   |
| GFM table                                 | `<table>` with `<th>`/`<td>` cells                                                              |
| `> quote`                                 | `<blockquote>`                                                                                  |
| `---`                                     | `<hr/>`                                                                                         |

### Language hints for code blocks

Common aliases are mapped to Confluence's language names

| Doc                | Confluence   |
|--------------------|--------------|
| `sh`/`shell`/`zsh` | `bash`       |
| `js`               | `javascript` |
| `py`               | `python`     |
| `yml`              | `yaml`       |
| `rb`               | `ruby`       |
| `c++`              | `cpp`        |
| `c#`               | `csharp`     |
| `objective-c`      | `objectivec` |

Anything else is passed through as-is.

### Dry-run

`--dry-run` is offline - it prints the page tree that would be synced without touching Confluence:

```bash
$ md2confluence sync --dry-run
[DRY-RUN] Space DOCS:
- Documentation Home  <- /abs/path/docs/docs.md
  - Installation  <- /abs/path/docs/installation/installation.md
    - Prerequisites  <- /abs/path/docs/installation/prerequisites.md
  - Guides  <- /abs/path/docs/guides/index.md
```

### List pages in a space

```bash
$ md2confluence list --space DOCS
Pages in space DOCS:

- Documentation Home             id=abc123 v5
  - Installation                 id=def456 v2
    - Prerequisites              new
```

## Troubleshooting

### Authentication errors (401)

- Verify your email and API token are correct. The token is **not** your Atlassian password.
- Generate a new token at [Atlassian Account -> Security](https://id.atlassian.com/manage-profile/security/api-tokens).
- Ensure the token has not been revoked.

### Orphaned pages after renaming

- If you rename a markdown file or its `# Heading`, md2confluence treats it as a **new** page (pages are matched by
  title). The old Confluence page remains until you delete it manually.
- Use `md2confluence list --space <KEY>` to see which local files map to existing pages.

### Parent page not found

- If `parentTitle` is set but the parent doesn't exist, md2confluence prints a warning and creates pages at the space
  root (the sync does **not** fail).
- Set `autoCreateParent: true` on the space to have md2confluence create an empty parent page on your behalf, or create
  it manually in Confluence first.

### Images not showing after sync

- Local image uploads run **after** the page is created/updated. The first render may briefly show broken images until
  the attachment request completes; reload the page.
- Images are looked up by basename - two different files with the same filename collide to one attachment. Rename one of
  them.

### Broken code block showing "Error loading the extension!"

- Confluence Cloud's Fabric editor is strict: the code macro requires `ac:schema-version="1"`, `ac:macro-id="{uuid}"`,
  hyphenated `ac:plain-text-body`, and CDATA-wrapped body content - we emit all of these. If you're still seeing an
  error, check your Confluence space permissions or report the generated storage XML.

## CI/CD Integration

Auth credentials are passed as env vars, so in CI you only need to commit a minimal config file with the `spaces` list -
secrets stay in the CI secret store.

A repo-side `md2confluence.yaml` only carries the `spaces` mapping (no secrets):

```yaml
spaces:
  - key: "DOCS"
    path: "./docs"
```

### Available image tags

The image is published to `ghcr.io/yousysadmin/md2confluence` (multi-arch: `linux/amd64` + `linux/arm64`, public).

| Tag                              | Source                                 | When to use                  |
|----------------------------------|----------------------------------------|------------------------------|
| `:vX.Y.Z`, `:latest`             | tag push -> goreleaser (`release.yml`) | Production. Reproducible.    |
| `:master`, `:master-<short-sha>` | every push to `master` (`docker.yml`)  | Tracking unreleased changes. |

A full ready-to-copy workflow showing all three integration styles below lives at [
`example/github-actions/sync-docs.yml`](example/github-actions/sync-docs.yml).

### Option A - Composite action (recommended)

Use the action wrapper at the repo root - shortest invocation, no `docker run` boilerplate:

```yaml
# .github/workflows/sync-docs.yml
name: Sync Docs to Confluence
on:
  push:
    branches: [ main ]
    paths: [ "docs/**", "md2confluence.yaml" ]
jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: yousysadmin/md2confluence@v1
        with:
          email: ${{ secrets.CONFLUENCE_EMAIL }}
          api-token: ${{ secrets.CONFLUENCE_API_TOKEN }}
          base-url: ${{ secrets.CONFLUENCE_BASE_URL }}
          config: ./md2confluence.yaml
          # Optional: --dry-run to preview without pushing.
          # args: --dry-run
          # Optional: pin a specific image (defaults to :latest).
          # image: ghcr.io/yousysadmin/md2confluence:v1.2.3
```

Action inputs: `email`, `api-token`, `base-url` (required), `config` (default `./md2confluence.yaml`), `command` (
default `sync`; also `upload` / `list`), `args`, `image`.

### Option B - Job-level container

Run every step in the job inside the image, with the repo mounted at `/work`. Useful when you want multiple
md2confluence steps in the same job:

```yaml
jobs:
  sync:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/yousysadmin/md2confluence:latest
    env:
      MD2CONFLUENCE_EMAIL: ${{ secrets.CONFLUENCE_EMAIL }}
      MD2CONFLUENCE_API_TOKEN: ${{ secrets.CONFLUENCE_API_TOKEN }}
      MD2CONFLUENCE_BASE_URL: ${{ secrets.CONFLUENCE_BASE_URL }}
    steps:
      - uses: actions/checkout@v4
      - run: md2confluence sync --config ./md2confluence.yaml
```

### Option C - `docker run` step

Use when the surrounding job is not containerized (e.g. needs other tools that aren't in the distroless image):

```yaml
- name: Sync to Confluence
  env:
    MD2CONFLUENCE_EMAIL: ${{ secrets.CONFLUENCE_EMAIL }}
    MD2CONFLUENCE_API_TOKEN: ${{ secrets.CONFLUENCE_API_TOKEN }}
    MD2CONFLUENCE_BASE_URL: ${{ secrets.CONFLUENCE_BASE_URL }}
  run: |
    docker run --rm \
      -v "$PWD:/work" -w /work \
      -e MD2CONFLUENCE_EMAIL -e MD2CONFLUENCE_API_TOKEN -e MD2CONFLUENCE_BASE_URL \
      ghcr.io/yousysadmin/md2confluence:latest \
      sync --config ./md2confluence.yaml
```

### Option D - Release tarball

Drop the binary onto the runner and call it directly - no Docker needed:

```yaml
- name: Install md2confluence
  run: |
    VERSION=1.0.0
    curl -sSL "https://github.com/yousysadmin/md2confluence/releases/download/v${VERSION}/md2confluence_${VERSION}_Linux_x86_64.tar.gz" \
      | tar -xz md2confluence
    install -m0755 md2confluence /usr/local/bin/
- name: Sync to Confluence
  run: md2confluence sync --config ./md2confluence.yaml
```
