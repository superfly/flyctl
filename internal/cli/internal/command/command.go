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
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/update"

	"github.com/superfly/flyctl/internal/cli/internal/app"
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

	// flush the cache to disk if required
	if c := cache.FromContext(ctx); c.Dirty() {
		path := filepath.Join(state.ConfigDirectory(ctx), cache.FileName)

		if err := c.Save(path); err != nil {
			logger.FromContext(ctx).
				Warnf("failed saving cache to %s: %v", path, err)
		}
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

// LoadAppConfigIfPresent is a Preparer which loads the application's
// configuration file from the path the user has selected via command line args
// or the current working directory.
func LoadAppConfigIfPresent(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)

	for _, path := range appConfigFilePaths(ctx) {
		switch cfg, err := app.LoadConfig(path); {
		case err == nil:
			logger.Debugf("app config loaded from %s", path)

			return app.WithConfig(ctx, cfg), nil // we loaded a configuration file
		case errors.Is(err, fs.ErrNotExist):
			logger.Debugf("no app config found at %s; skipped.", path)

			continue
		default:
			return nil, fmt.Errorf("failed loading app config from %s: %w", path, err)
		}
	}

	return ctx, nil
}

// appConfigFilePaths returns the possible paths at which we may find a fly.toml
// in order of preference. it takes into consideration whether the user has
// specified a command-line path to a config file.
func appConfigFilePaths(ctx context.Context) (paths []string) {
	if p := flag.GetAppConfigFilePath(ctx); p != "" {
		paths = append(paths, p, filepath.Join(p, app.DefaultConfigFileName))

		return
	}

	wd := state.WorkingDirectory(ctx)
	paths = append(paths, filepath.Join(wd, app.DefaultConfigFileName))

	return
}

var errRequireAppName = fmt.Errorf("we couldn't find a fly.toml nor an app specified by the -a flag. If you want to launch a new app, use '%s launch'", buildinfo.Name())

// RequireAppName is a Preparer which makes sure the user has selected an
// application name via command line arguments, the environment or an application
// config file (fly.toml). It embeds LoadAppConfigIfPresent.
func RequireAppName(ctx context.Context) (context.Context, error) {
	ctx, err := LoadAppConfigIfPresent(ctx)
	if err != nil {
		return nil, err
	}

	name := flag.GetApp(ctx)
	if name == "" {
		// if there's no flag present, first consult with the environment
		if name = env.First("FLY_APP"); name == "" {
			// and then with the config file (if any)
			if cfg := app.ConfigFromContext(ctx); cfg != nil {
				name = cfg.AppName
			}
		}
	}

	if name == "" {
		return nil, errRequireAppName
	}

	return app.WithName(ctx, name), nil
}

// LoadAppNameIfPresent is a Preparer which adds app name if the user has used --app or there appConfig
// but unlike RequireAppName it does not error if the user has not specified an app name.
func LoadAppNameIfPresent(ctx context.Context) (context.Context, error) {
	ctx, err := LoadAppConfigIfPresent(ctx)
	if err != nil {
		return nil, err
	}

	name := flag.GetApp(ctx)
	if name == "" {
		// if there's no flag present, first consult with the environment
		if name = env.First("FLY_APP"); name == "" {
			// and then with the config file (if any)
			if cfg := app.ConfigFromContext(ctx); cfg != nil {
				name = cfg.AppName
			}
		}
	}

	return app.WithName(ctx, name), nil
}

// RequireRegion is a Preparer which makes sure the user has selected a region via command line arguments,
func RequireRegion(ctx context.Context) (context.Context, error) {
	ctx, err := LoadAppConfigIfPresent(ctx)
	if err != nil {
		return nil, err
	}

	region := flag.GetRegion(ctx)
	if region == "" {
		return nil, fmt.Errorf("region is required")
	}

	return ctx, nil
}
