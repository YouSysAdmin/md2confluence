# Installation

## From a release archive

Download the archive matching your platform from the project's GitHub
**Releases** page, extract it, and put the binary somewhere on your
`$PATH`.

```bash
tar -xzf md2confluence_Linux_x86_64.tar.gz
sudo mv md2confluence /usr/local/bin/
md2confluence --version
```

## From source

Requires a recent Go toolchain.

```bash
git clone <repo>
cd md2confluence
go build -o md2confluence ./cmd/md2confluence
```

## Verify

```bash
md2confluence --help
```

You should see the `sync`, `upload`, and `list` subcommands.
