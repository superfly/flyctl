// Package cmd implements helpers useful to commands.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/update"

	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/state"
)

type (
	Preparer func(context.Context) (context.Context, error)

	Runner func(context.Context) error
)

func New(dsk string, fn Runner, p ...Preparer) *cobra.Command {
	ds := docstrings.Get(dsk)

	return Build(ds.Usage, ds.Short, ds.Long, fn, p...)
}

func Build(usage, short, long string, fn Runner, p ...Preparer) *cobra.Command {
	return &cobra.Command{
		Use:   usage,
		Short: short,
		Long:  long,
		RunE:  newRunE(fn, p...),
	}
}

var commonPreparers = []Preparer{
	determineWorkingDir,
	determineUserHomeDir,
	determineConfigDir,
	determineConfigFile,
	loadConfig,
	initClient,
	promptToUpdate,
}

func newRunE(fn Runner, preparers ...Preparer) func(*cobra.Command, []string) error {
	if fn == nil {
		return nil
	}

	return func(cmd *cobra.Command, _ []string) (err error) {
		ctx := cmd.Context()
		ctx = NewContext(ctx, cmd)
		ctx = flag.NewContext(ctx, cmd.Flags())

		// run the common preparers
		if ctx, err = prepare(ctx, commonPreparers...); err != nil {
			return
		}

		// run the preparers specific to the command
		if ctx, err = prepare(ctx, preparers...); err == nil {
			// and run the command
			err = fn(ctx)
		}

		return
	}
}

func prepare(parent context.Context, preparers ...Preparer) (ctx context.Context, err error) {
	ctx = parent

	for _, p := range preparers {
		if ctx, err = p(ctx); err != nil {
			break
		}
	}

	return
}

func promptToUpdate(ctx context.Context) (context.Context, error) {
	update.PromptFor(ctx, iostreams.FromContext(ctx))

	return ctx, nil
}

func determineWorkingDir(ctx context.Context) (context.Context, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error determining working directory: %w", err)
	}

	logger.FromContext(ctx).
		Debugf("determined working directory: %q", wd)

	return state.WithWorkingDirectory(ctx, wd), nil
}

func determineUserHomeDir(ctx context.Context) (context.Context, error) {
	wd, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error determining user home directory: %w", err)
	}

	logger.FromContext(ctx).
		Debugf("determined user home directory: %q", wd)

	return state.WithUserHomeDirectory(ctx, wd), nil
}

func determineConfigDir(ctx context.Context) (context.Context, error) {
	dir := filepath.Join(state.UserHomeDirectory(ctx), ".fly")

	logger.FromContext(ctx).
		Debugf("determined config directory: %q", dir)

	return state.WithConfigDirectory(ctx, dir), nil
}

func determineConfigFile(ctx context.Context) (context.Context, error) {
	dir := filepath.Join(state.ConfigDirectory(ctx), "config.yml")

	logger.FromContext(ctx).
		Debugf("determined config file: %q", dir)

	return state.WithConfigFile(ctx, dir), nil
}

func loadConfig(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)

	cfg := config.New()

	// first apply the environment
	cfg.ApplyEnv()

	// then the file (if any)
	path := state.ConfigFile(ctx)
	switch err := cfg.ApplyFile(path); {
	case err == nil:
		// config file does not exist exists
	case errors.Is(err, fs.ErrNotExist):
		logger.Warnf("no config file found at %s", path)
	default:
		return nil, err
	}

	// and lastly apply command line flags
	cfg.ApplyFlags(flag.FromContext(ctx))

	logger.Debug("config initialized.")

	return config.NewContext(ctx, cfg), nil
}

func initClient(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)
	cfg := config.FromContext(ctx)

	// TODO: refactor so that api package does NOT depend on global state
	api.SetBaseURL(cfg.APIBaseURL)
	api.SetErrorLog(cfg.LogGQLErrors)

	c := client.FromToken(cfg.AccessToken)
	logger.Debug("client initialized.")

	return client.NewContext(ctx, c), nil
}

// RequireSession is a preparare which makes sure a session exists.
func RequireSession(ctx context.Context) (context.Context, error) {
	if !client.FromContext(ctx).Authenticated() {
		return nil, client.ErrNoAuthToken
	}

	return ctx, nil
}
