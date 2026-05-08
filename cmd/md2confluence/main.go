package main

import (
	"os"

	"github.com/yousysadmin/md2confluence/internal/cli"
)

// Populated by goreleaser via -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := cli.Execute(cli.BuildInfo{Version: version, Commit: commit, Date: date}); err != nil {
		os.Exit(1)
	}
}
