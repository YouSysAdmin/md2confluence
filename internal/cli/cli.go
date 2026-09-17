// Package cli wires up the cobra commands and contains the orchestration
// logic for the md2confluence CLI. The cmd/md2confluence/main.go entry point
// only injects build metadata, sets up signal handling, and calls Execute.
package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yousysadmin/md2confluence/internal/config"
	"github.com/yousysadmin/md2confluence/internal/confluence"
)

// BuildInfo build metadata supplied by main (populated via goreleaser ldflags).
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Process exit codes following Unix conventions.
const (
	ExitOK          = 0
	ExitError       = 1   // runtime failure
	ExitUsage       = 2   // bad flags or arguments
	ExitInterrupted = 130 // 128 + SIGINT
)

// pageClient is the subset of *confluence.Client the commands depend on.
// Tests substitute an in-memory fake.
type pageClient interface {
	LookupSpaceID(ctx context.Context, key string) (string, error)
	SearchPage(ctx context.Context, spaceID, title string) (*confluence.Page, error)
	CreatePage(ctx context.Context, req *confluence.CreateRequest) (*confluence.Page, error)
	UpdatePage(ctx context.Context, id string, req *confluence.UpdateRequest) (*confluence.Page, error)
	UploadAttachment(ctx context.Context, pageID, filePath string) error
}

// clientFactory builds the API client from loaded config.
type clientFactory func(cfg *config.Config) pageClient

func defaultClientFactory(cfg *config.Config) pageClient {
	return confluence.NewClient(cfg.BaseURL, cfg.Email, cfg.APIToken)
}

// app holds per-Execute state: flag bindings and injectable dependencies.
// There are no package-level globals so tests can run commands in parallel.
type app struct {
	cfgFile   string
	dryRun    bool
	space     string
	output    string
	newClient clientFactory
}

// usageError marks an error caused by invalid flags or arguments so main can
// exit with ExitUsage instead of ExitError.
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// Execute builds the root command tree and runs it with ctx. Cancelling ctx
// (e.g. on SIGINT) aborts in-flight HTTP requests and the tree walk.
func Execute(ctx context.Context, b BuildInfo) error {
	return newRootCmd(b, defaultClientFactory).ExecuteContext(ctx)
}

// ExitCode maps an error returned by Execute to a process exit code.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, context.Canceled):
		return ExitInterrupted
	case isUsageError(err):
		return ExitUsage
	default:
		return ExitError
	}
}

func isUsageError(err error) bool {
	var ue *usageError
	if errors.As(err, &ue) {
		return true
	}
	// Cobra returns these without a hook point to wrap them.
	msg := err.Error()
	return strings.HasPrefix(msg, "unknown command") ||
		strings.HasPrefix(msg, "required flag(s)") ||
		strings.HasPrefix(msg, "if any flags in the group")
}

// usageArgs wraps a cobra positional-argument validator so its failures map
// to ExitUsage.
func usageArgs(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			return &usageError{err}
		}
		return nil
	}
}

func newRootCmd(b BuildInfo, newClient clientFactory) *cobra.Command {
	a := &app{newClient: newClient}

	root := &cobra.Command{
		Use:           "md2confluence",
		Short:         "Sync markdown files to Confluence pages",
		Version:       fmt.Sprintf("%s (commit %s, built %s)", b.Version, b.Commit, b.Date),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if _, err := parseOutputFormat(a.output); err != nil {
				return &usageError{err}
			}
			return nil
		},
	}
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &usageError{err}
	})

	pf := root.PersistentFlags()
	pf.StringVarP(&a.cfgFile, "config", "c", "", "config file (default $HOME/.config/md2confluence/config.yaml)")
	pf.StringVarP(&a.output, "output", "o", string(formatText), "output format: text|json")
	_ = root.RegisterFlagCompletionFunc("output", cobra.FixedCompletions(outputFormatNames(), cobra.ShellCompDirectiveNoFileComp))

	root.AddCommand(
		newSyncCmd(a),
		newUploadCmd(a),
		newListCmd(a),
	)

	return root
}

// loadConfig reads and validates config, then returns it with a ready client.
func (a *app) loadConfig() (*config.Config, pageClient, error) {
	cfg, err := config.Load(a.cfgFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, a.newClient(cfg), nil
}

// addSpaceFlag registers --space/-s with completion sourced from the config
// file's space keys.
func (a *app) addSpaceFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().StringVarP(&a.space, "space", "s", "", usage)
	_ = cmd.RegisterFlagCompletionFunc("space", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cfg, err := config.Read(a.cfgFile)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		keys := make([]string, 0, len(cfg.Spaces))
		for _, sp := range cfg.Spaces {
			if strings.HasPrefix(sp.Key, toComplete) {
				keys = append(keys, sp.Key)
			}
		}
		return keys, cobra.ShellCompDirectiveNoFileComp
	})
}
