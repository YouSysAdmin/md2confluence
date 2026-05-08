# Getting Started

Two steps to your first sync:

1. Install the CLI - see **Installation**.
2. Configure credentials and your space mapping - see **Configuration**.

Once both are in place, a first-run smoke test:

```bash
md2confluence sync --config ./example/config.yaml --dry-run
```

`--dry-run` is entirely offline; it prints the page tree resolved from
your directory structure without hitting Confluence. When the tree looks
right, drop the flag to perform the real sync.
