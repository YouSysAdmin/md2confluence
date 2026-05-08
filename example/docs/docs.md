# Documentation Home

Welcome to the example documentation tree. This page is generated from
`example/docs/docs.md` and serves as the root of the space. Every
subdirectory under it becomes a child page.

## Sections

- **Getting Started** - install the CLI and configure a space mapping.
- **Reference** - a showcase of supported markdown syntax and how each
  element maps to Confluence storage format.

## How this tree maps to Confluence

```
example/docs/
├── docs.md                              → "Documentation Home"
├── getting-started/
│   ├── getting-started.md               → "Getting Started" (section body)
│   ├── installation.md                  → child
│   └── configuration.md                 → child
└── reference/
    ├── reference.md                     → "Reference" (section body)
    └── markdown-examples.md             → child
```

Run it yourself:

```bash
md2confluence sync --config ./example/config.yaml --dry-run
```
