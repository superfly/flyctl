// Package command implements helpers useful for when building cobra commands.
package command

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/blang/semver"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/update"

	"github.com/superfly/flyctl/internal/cli/internal/cache"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/cli/internal/task"
)

type (
	Preparer func(context.Context) (context.Context, error)

	Runner func(context.Context) error
)

// TODO: remove once all commands are implemented.
var ErrNotImplementedYet = errors.New("command not implemented yet")

func New(usage, short, long string, fn Runner, p ...Preparer) *cobra.Command {
	return &cobra.Command{
		Use:   usage,
		Short: short,
		Long:  long,
		RunE:  newRunE(fn, p...),
	}
}

var commonPreparers = []Preparer{
	determineHostname,
	determineWorkingDir,
	determineUserHomeDir,
	determineConfigDir,
	loadCache,
	loadConfig,
	initTaskManager,
	startQueryingForNewRelease,
	promptToUpdate,
	initClient,
}

// TODO: remove after migration is complete
func WrapRunE(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) (err error) {
		ctx := cmd.Context()
		ctx = NewContext(ctx, cmd)
		ctx = flag.NewContext(ctx, cmd.Flags())

		// run the common preparers
		if ctx, err = prepare(ctx, commonPreparers...); err != nil {
			return
		}

		err = fn(cmd, args)

		// and the
		finalize(ctx)

		return
	}
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
		if ctx, err = prepare(ctx, preparers...); err != nil {
			return
		}

		// run the command
		if err = fn(ctx); err == nil {
			// and finally, run the finalizer
			finalize(ctx)
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

func finalize(ctx context.Context) {
	// shutdown async tasks
	task.FromContext(ctx).Shutdown()

	// and flush the cache to disk if required
	c := cache.FromContext(ctx)
	if !c.Dirty() {
		return
	}

	path := filepath.Join(state.ConfigDirectory(ctx), cache.FileName)
	if err := c.Save(path); err != nil {
		logger.FromContext(ctx).
			Warnf("failed saving cache to %s: %v", path, err)
	}
}

func determineHostname(ctx context.Context) (context.Context, error) {
	h, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed determining hostname: %w", err)
	}

	logger.FromContext(ctx).
		Debugf("determined hostname: %q", h)

	return state.WithHostname(ctx, h), nil
}

func determineWorkingDir(ctx context.Context) (context.Context, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed determining working directory: %w", err)
	}

	logger.FromContext(ctx).
		Debugf("determined working directory: %q", wd)

	return state.WithWorkingDirectory(ctx, wd), nil
}

func determineUserHomeDir(ctx context.Context) (context.Context, error) {
	wd, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed determining user home directory: %w", err)
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

func loadCache(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)

	path := filepath.Join(state.ConfigDirectory(ctx), cache.FileName)

	c, err := cache.Load(path)
	if err != nil {
		c = cache.New()

		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warnf("failed loading cache file from %s: %v", path, err)
		}
	}

	logger.Debug("cache loaded.")

	return cache.NewContext(ctx, c), nil
}

func loadConfig(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)

	cfg := config.New()

	// Apply config from the config file, if it exists
	path := filepath.Join(state.ConfigDirectory(ctx), config.FileName)
	switch err := cfg.ApplyFile(path); {
	case err == nil:
		// config file loaded
	case errors.Is(err, fs.ErrNotExist):
		logger.Warnf("no config file found at %s", path)
	default:
		return nil, err
	}

	// Apply config from the environment, overriding anything from the file
	cfg.ApplyEnv()

	// Finally, apply command line options, overriding any previous setting
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

func initTaskManager(ctx context.Context) (context.Context, error) {
	tm := task.New(ctx)

	logger.FromContext(ctx).Debug("initialized task manager.")

	return task.NewContext(ctx, tm), nil
}

func startQueryingForNewRelease(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)

	cache := cache.FromContext(ctx)
	if !update.Check() || time.Since(cache.LastCheckedAt()) < time.Hour {
		logger.Debug("skipped querying for new release")

		return ctx, nil
	}

	channel := cache.Channel()
	tm := task.FromContext(ctx)

	tm.Run(func(parent context.Context) {
		ctx, cancel := context.WithTimeout(parent, time.Second)
		defer cancel()

		switch r, err := update.LatestRelease(ctx, channel); {
		case err == nil:
			if r == nil {
				break
			}

			cache.SetLatestRelease(channel, r)

			logger.Debugf("querying for release resulted to %v", r.Version)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			break
		default:
			logger.Warnf("failed querying for new release: %v", err)
		}
	})

	logger.Debug("started querying for new release")

	return ctx, nil
}

func promptToUpdate(ctx context.Context) (context.Context, error) {
	if !update.Check() {
		return ctx, nil
	}

	c := cache.FromContext(ctx)

	r := c.LatestRelease()
	if r == nil {
		return ctx, nil
	}

	logger := logger.FromContext(ctx)

	current := buildinfo.Info().Version

	switch latest, err := semver.ParseTolerant(r.Version); {
	case err != nil:
		logger.Warnf("error parsing version number '%s': %s", r.Version, err)

		return ctx, nil
	case latest.LTE(current):
		return ctx, nil
	}

	msg := fmt.Sprintf("Update available %s -> %s.\nRun \"%s\" to upgrade.",
		current,
		r.Version,
		aurora.Bold(buildinfo.Name()+" version update"),
	)

	stderr := iostreams.FromContext(ctx).ErrOut
	fmt.Fprintln(stderr, aurora.Yellow(msg))

	return ctx, nil
}

// RequireSession is a Preparer which makes sure a session exists.
func RequireSession(ctx context.Context) (context.Context, error) {
	if !client.FromContext(ctx).Authenticated() {
		return nil, client.ErrNoAuthToken
	}

	return ctx, nil
}
