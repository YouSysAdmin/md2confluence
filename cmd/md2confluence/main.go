package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yousysadmin/md2confluence/internal/cli"
)

// Populated by goreleaser via -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := cli.Execute(ctx, cli.BuildInfo{Version: version, Commit: commit, Date: date})
	stop()
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "Interrupted.")
	} else {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
	os.Exit(cli.ExitCode(err))
}
