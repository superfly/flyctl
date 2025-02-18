// Package command implements helpers useful for when building cobra commands.
package command

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/auth/webauth"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/cmdutil/preparers"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/incidents"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/internal/update"
	"github.com/superfly/flyctl/internal/version"
)

type Runner func(context.Context) error

func New(usage, short, long string, fn Runner, p ...preparers.Preparer) *cobra.Command {
	return &cobra.Command{
		Use:   usage,
		Short: short,
		Long:  long,
		RunE:  newRunE(fn, p...),
	}
}

// Preparers are split between here and the preparers package because
// tab-completion needs to run *some* of them, and importing this package from there
// would create a circular dependency. Likewise, if *all* the preparers were in the preparers module,
// that would also create a circular dependency.
// I don't like this, but it's shippable until someone else fixes it
var commonPreparers = []preparers.Preparer{
	preparers.ApplyAliases,
	determineHostname,
	determineWorkingDir,
	preparers.DetermineConfigDir,
	ensureConfigDirExists,
	ensureConfigDirPerms,
	loadCache,
	preparers.LoadConfig,
	startQueryingForNewRelease,
	promptAndAutoUpdate,
	startMetrics,
	notifyStatuspageIncidents,
}

var authPreparers = []preparers.Preparer{
	preparers.InitClient,
	killOldAgent,
	notifyHostIssues,
}

func sendOsMetric(ctx context.Context, state string) {
	// Send /runs/[os_name]/[state]
	osName := ""
	switch runtime.GOOS {
	case "darwin":
		osName = "macos"
	case "linux":
		osName = "linux"
	case "windows":
		osName = "windows"
	default:
		osName = "other"
	}
	metrics.SendNoData(ctx, fmt.Sprintf("runs/%s/%s", osName, state))
}

func newRunE(fn Runner, preparers ...preparers.Preparer) func(*cobra.Command, []string) error {
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

		// run the preparers that perform or require authorization
		if ctx, err = prepare(ctx, authPreparers...); err != nil {
			return
		}

		// run the preparers specific to the command
		if ctx, err = prepare(ctx, preparers...); err != nil {
			return
		}

		// start task manager using the prepared context
		task.FromContext(ctx).Start(ctx)

		sendOsMetric(ctx, "started")
		task.FromContext(ctx).RunFinalizer(func(ctx context.Context) {
			io := iostreams.FromContext(ctx)

			if !metrics.IsFlushMetricsDisabled(ctx) {
				err := metrics.FlushMetrics(ctx)
				if err != nil {
					fmt.Fprintln(io.ErrOut, "Error spawning metrics process: ", err)
				}
			}
		})

		defer func() {
			if err == nil {
				sendOsMetric(ctx, "successful")
			}
		}()

		// run the command
		if err = fn(ctx); err == nil {
			// and finally, run the finalizer
			finalize(ctx)
		}

		return
	}
}

func prepare(parent context.Context, preparers ...preparers.Preparer) (ctx context.Context, err error) {
	ctx = parent

	for _, p := range preparers {
		if ctx, err = p(ctx); err != nil {
			break
		}
	}

	return
}

func finalize(ctx context.Context) {
	// todo[md] move this to a background task
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

func ensureConfigDirExists(ctx context.Context) (context.Context, error) {
	dir := state.ConfigDirectory(ctx)

	switch fi, err := os.Stat(dir); {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("failed creating config directory: %w", err)
		}
	case err != nil:
		return nil, fmt.Errorf("failed stat-ing config directory: %w", err)
	case !fi.IsDir():
		return nil, fmt.Errorf("the path to the config directory (%s) is occupied by not a directory", dir)
	}

	logger.FromContext(ctx).
		Debug("ensured config directory exists.")

	return ctx, nil
}

func ensureConfigDirPerms(parent context.Context) (ctx context.Context, err error) {
	defer func() {
		if err != nil {
			ctx = nil
			err = fmt.Errorf("failed ensuring config directory perms: %w", err)

			return
		}

		logger.FromContext(ctx).
			Debug("ensured config directory perms.")
	}()

	ctx = parent
	dir := state.ConfigDirectory(parent)

	var f *os.File
	if f, err = os.CreateTemp(dir, "perms.*"); err != nil {
		return
	}
	defer func() {
		if e := os.Remove(f.Name()); err == nil {
			err = e
		}
	}()

	err = f.Close()

	return
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

func startQueryingForNewRelease(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)

	cache := cache.FromContext(ctx)
	if !update.Check() || time.Since(cache.LastCheckedAt()) < time.Hour {
		logger.Debug("skipped querying for new release")

		return ctx, nil
	}

	channel := cache.Channel()

	queryRelease := func(parent context.Context) {
		logger.Debug("started querying for new release")

		ctx, cancel := context.WithTimeout(parent, time.Second)
		defer cancel()

		switch r, err := update.LatestRelease(ctx, channel); {
		case err == nil:
			if r == nil {
				break
			}

			// The API won't return yanked versions, but we don't have a good way
			// to yank homebrew releases. If we're under homebrew, we'll validate through the API
			if update.IsUnderHomebrew() {
				if relErr := update.ValidateRelease(ctx, r.Version); relErr != nil {
					logger.Debugf("latest release %s is invalid: %v", r.Version, relErr)
					break
				}
			}

			cache.SetLatestRelease(channel, r)

			// Check if the current version has been yanked.
			if cache.IsCurrentVersionInvalid() == "" {
				currentRelErr := update.ValidateRelease(ctx, buildinfo.Version().String())
				if currentRelErr != nil {
					var invalidRelErr *update.InvalidReleaseError
					if errors.As(currentRelErr, &invalidRelErr) {
						cache.SetCurrentVersionInvalid(invalidRelErr)
					}
				}
			}

			logger.Debugf("querying for release resulted to %v", r.Version)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			break
		default:
			logger.Warnf("failed querying for new release: %v", err)
		}
	}

	// If it's been more than a week since we've checked for a new release,
	// check synchronously. Otherwise, check asynchronously.
	if time.Since(cache.LastCheckedAt()) > (24 * time.Hour * 7) {
		queryRelease(ctx)
	} else {
		task.FromContext(ctx).Run(queryRelease)
	}

	return ctx, nil
}

// shouldIgnore allows a preparer to disable itself for specific commands
// E.g. `shouldIgnore([][]string{{"version", "upgrade"}, {"machine", "status"}})`
// would return true for "fly version upgrade" and "fly machine status"
func shouldIgnore(ctx context.Context, cmds [][]string) bool {
	cmd := FromContext(ctx)

	for _, ignoredCmd := range cmds {
		match := true
		currentCmd := cmd
		// The shape of the ignoredCmd slice is something like ["version", "upgrade"],
		// but we're walking up the tree from the end, so we have to iterate that in reverse
		for i := len(ignoredCmd) - 1; i >= 0; i-- {
			if !currentCmd.HasParent() || currentCmd.Use != ignoredCmd[i] {
				match = false
				break
			}
			currentCmd = currentCmd.Parent()
		}
		// Ensure that we have the root node, so that a filter on ["y"] wouldn't be tripped by "fly x y"
		if match {
			if !currentCmd.HasParent() {
				return true
			}
		}
	}
	return false
}

func promptAndAutoUpdate(ctx context.Context) (context.Context, error) {
	cfg := config.FromContext(ctx)
	if shouldIgnore(ctx, [][]string{
		{"version"},
		{"version", "upgrade"},
		{"settings", "autoupdate"},
	}) {
		return ctx, nil
	}

	logger.FromContext(ctx).Debug("checking for updates...")

	if !update.Check() {
		return ctx, nil
	}

	var (
		current   = buildinfo.Version()
		cache     = cache.FromContext(ctx)
		logger    = logger.FromContext(ctx)
		io        = iostreams.FromContext(ctx)
		colorize  = io.ColorScheme()
		latestRel = cache.LatestRelease()
		silent    = cfg.JSONOutput
	)

	if latestRel == nil {
		return ctx, nil
	}

	versionInvalidMsg := cache.IsCurrentVersionInvalid()
	if versionInvalidMsg != "" && !silent {
		fmt.Fprintf(io.ErrOut, "The current version of flyctl is invalid: %s\n", versionInvalidMsg)
	}

	latest, err := version.Parse(latestRel.Version)
	if err != nil {
		logger.Warnf("error parsing version number '%s': %s", latestRel.Version, err)
		return ctx, err
	}

	if !latest.Newer(current) {
		if versionInvalidMsg != "" && !silent {
			// Continuing from versionInvalidMsg above
			fmt.Fprintln(io.ErrOut, "but there is not a newer version available. Proceed with caution!")
		}
		return ctx, nil
	}

	promptForUpdate := false

	// The env.IsCI check is technically redundant (it should be done in update.Check), but
	// it's nice to be extra cautious.
	if cfg.AutoUpdate && !env.IsCI() && update.CanUpdateThisInstallation() {
		if versionInvalidMsg != "" || current.SignificantlyBehind(latest) {
			if !silent {
				fmt.Fprintln(io.ErrOut, colorize.Green(fmt.Sprintf("Automatically updating %s -> %s.", current, latestRel.Version)))
			}

			err := update.UpgradeInPlace(ctx, io, latestRel.Prerelease, silent)
			if err != nil {
				return nil, fmt.Errorf("failed to update, and the current version is severely out of date: %w", err)
			}
			// Does not return on success
			err = update.Relaunch(ctx, silent)
			return nil, fmt.Errorf("failed to relaunch after updating: %w", err)
		} else if runtime.GOOS != "windows" {
			// Background auto-update has terrible UX on windows,
			// with flickery powershell progress bars and UAC prompts.
			// For Windows, we just prompt for updates, and only auto-update when severely outdated (the before-command update)
			if err := update.BackgroundUpdate(); err != nil {
				fmt.Fprintf(io.ErrOut, "failed to autoupdate: %s\n", err)
			} else {
				promptForUpdate = false
			}
		}
	}
	if !silent {
		if !cfg.AutoUpdate && versionInvalidMsg != "" {
			// Continuing from versionInvalidMsg above
			fmt.Fprintln(io.ErrOut, "Proceed with caution!")
		}
		if promptForUpdate {
			fmt.Fprintln(io.ErrOut, colorize.Yellow(fmt.Sprintf("Update available %s -> %s.", current, latestRel.Version)))
			fmt.Fprintln(io.ErrOut, colorize.Yellow(fmt.Sprintf("Run \"%s\" to upgrade.", colorize.Bold(buildinfo.Name()+" version upgrade"))))
		}
	}

	return ctx, nil
}

func killOldAgent(ctx context.Context) (context.Context, error) {
	path := filepath.Join(state.ConfigDirectory(ctx), "agent.pid")

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ctx, nil // no old agent running or can't access that file
	} else if err != nil {
		return nil, fmt.Errorf("failed reading old agent's PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed determining old agent's PID: %w", err)
	}

	logger := logger.FromContext(ctx)
	unlink := func() (err error) {
		if err = os.Remove(path); err != nil {
			err = fmt.Errorf("failed removing old agent's PID file: %w", err)

			return
		}

		logger.Debug("removed old agent's PID file.")

		return
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving old agent's process: %w", err)
	} else if p == nil {
		return ctx, unlink()
	}

	if err := p.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return nil, fmt.Errorf("failed killing old agent process: %w", err)
	}

	logger.Debugf("killed old agent (PID: %d)", pid)

	if err := unlink(); err != nil {
		return nil, err
	}

	time.Sleep(time.Second) // we've killed and removed the pid file

	return ctx, nil
}

func startMetrics(ctx context.Context) (context.Context, error) {
	metrics.RecordCommandContext(ctx)

	task.FromContext(ctx).RunFinalizer(func(ctx context.Context) {
		metrics.FlushPending()
	})

	return ctx, nil
}

func notifyStatuspageIncidents(ctx context.Context) (context.Context, error) {
	if shouldIgnore(ctx, [][]string{
		{"incidents", "list"},
	}) {
		return ctx, nil
	}

	if !incidents.Check() {
		return ctx, nil
	}

	incidents.QueryStatuspageIncidents(ctx)

	return ctx, nil
}

func notifyHostIssues(ctx context.Context) (context.Context, error) {
	if shouldIgnore(ctx, [][]string{
		{"incidents", "hosts", "list"},
	}) {
		return ctx, nil
	}

	if !incidents.Check() {
		return ctx, nil
	}

	appCtx, err := LoadAppNameIfPresent(ctx)
	if err == nil {
		incidents.QueryHostIssues(appCtx)
	}

	return ctx, nil
}

func ExcludeFromMetrics(ctx context.Context) (context.Context, error) {
	metrics.Enabled = false
	return ctx, nil
}

// RequireSession is a Preparer which makes sure a session exists.
func RequireSession(ctx context.Context) (context.Context, error) {
	if !flyutil.ClientFromContext(ctx).Authenticated() {
		io := iostreams.FromContext(ctx)
		// Ensure we have a session, and that the user hasn't set any flags that would lead them to expect consistent output or a lack of prompts
		if io.IsInteractive() &&
			!env.IsCI() &&
			!flag.GetBool(ctx, "now") &&
			!flag.GetBool(ctx, "json") &&
			!flag.GetBool(ctx, "quiet") &&
			!flag.GetBool(ctx, "yes") {

			// Ask before we start opening things
			confirmed, err := prompt.Confirm(ctx, "You must be logged in to do this. Would you like to sign in?")
			if err != nil {
				return nil, err
			}
			if !confirmed {
				return nil, fly.ErrNoAuthToken
			}

			// Attempt to log the user in
			token, err := webauth.RunWebLogin(ctx, false)
			if err != nil {
				return nil, err
			}
			if err := webauth.SaveToken(ctx, token); err != nil {
				return nil, err
			}

			// Reload the config
			logger.FromContext(ctx).Debug("reloading config after login")
			if ctx, err = prepare(ctx, preparers.LoadConfig); err != nil {
				return nil, err
			}

			// first reset the client
			ctx = flyutil.NewContextWithClient(ctx, nil)

			// Re-run the auth preparers to update the client with the new token
			logger.FromContext(ctx).Debug("re-running auth preparers after login")
			if ctx, err = prepare(ctx, authPreparers...); err != nil {
				return nil, err
			}
		} else {
			return nil, fly.ErrNoAuthToken
		}
	}

	config.MonitorTokens(ctx, config.Tokens(ctx), tryOpenUserURL)

	return ctx, nil
}

// Apply uiex client to uiex
func RequireUiex(ctx context.Context) (context.Context, error) {
	cfg := config.FromContext(ctx)

	if uiexutil.ClientFromContext(ctx) == nil {
		client, err := uiexutil.NewClientWithOptions(ctx, uiex.NewClientOpts{
			Logger: logger.FromContext(ctx),
			Tokens: cfg.Tokens,
		})
		if err != nil {
			return nil, err
		}
		ctx = uiexutil.NewContextWithClient(ctx, client)
	}

	return ctx, nil
}

func tryOpenUserURL(ctx context.Context, url string) error {
	io := iostreams.FromContext(ctx)

	if !io.IsInteractive() || env.IsCI() {
		return errors.New("failed opening browser")
	}

	if err := open.Run(url); err != nil {
		fmt.Fprintf(io.ErrOut, "failed opening browser. Copy the url (%s) into a browser and continue\n", url)
	}

	return nil
}

// LoadAppConfigIfPresent is a Preparer which loads the application's
// configuration file from the path the user has selected via command line args
// or the current working directory.
func LoadAppConfigIfPresent(ctx context.Context) (context.Context, error) {
	// Shortcut to avoid unmarshaling and querying Web when
	// LoadAppConfigIfPresent is chained with RequireAppName
	if cfg := appconfig.ConfigFromContext(ctx); cfg != nil {
		metrics.IsUsingGPU = cfg.IsUsingGPU()
		return ctx, nil
	}

	logger := logger.FromContext(ctx)
	for _, path := range appConfigFilePaths(ctx) {
		switch cfg, err := appconfig.LoadConfig(path); {
		case err == nil:
			logger.Debugf("app config loaded from %s", path)
			if err := cfg.SetMachinesPlatform(); err != nil {
				logger.Warnf("WARNING the config file at '%s' is not valid: %s", path, err)
			}
			metrics.IsUsingGPU = cfg.IsUsingGPU()
			return appconfig.WithConfig(ctx, cfg), nil // we loaded a configuration file
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
		paths = append(paths, p, filepath.Join(p, appconfig.DefaultConfigFileName))

		return
	}

	wd := state.WorkingDirectory(ctx)
	paths = append(paths,
		filepath.Join(wd, appconfig.DefaultConfigFileName),
		filepath.Join(wd, strings.Replace(appconfig.DefaultConfigFileName, ".toml", ".json", 1)),
		filepath.Join(wd, strings.Replace(appconfig.DefaultConfigFileName, ".toml", ".yaml", 1)),
	)

	return
}

var ErrRequireAppName = fmt.Errorf("the config for your app is missing an app name, add an app field to the fly.toml file or specify with the -a flag")

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
			if cfg := appconfig.ConfigFromContext(ctx); cfg != nil {
				name = cfg.AppName
			}
		}
	}

	if name == "" {
		return nil, ErrRequireAppName
	}

	return appconfig.WithName(ctx, name), nil
}

// RequireAppNameNoFlag is a Preparer which makes sure the user has selected an
// application name via the environment or an application
// config file (fly.toml). It embeds LoadAppConfigIfPresent.
//
// Identical to RequireAppName but does not check for the --app flag.
func RequireAppNameNoFlag(ctx context.Context) (context.Context, error) {
	ctx, err := LoadAppConfigIfPresent(ctx)
	if err != nil {
		return nil, err
	}

	// First consult with the environment
	name := env.First("FLY_APP")
	if name == "" {
		// and then with the config file (if any)
		if cfg := appconfig.ConfigFromContext(ctx); cfg != nil {
			name = cfg.AppName
		}
	}

	if name == "" {
		return nil, ErrRequireAppName
	}

	return appconfig.WithName(ctx, name), nil
}

// LoadAppNameIfPresent is a Preparer which adds app name if the user has used --app or there appConfig
// but unlike RequireAppName it does not error if the user has not specified an app name.
func LoadAppNameIfPresent(ctx context.Context) (context.Context, error) {
	localCtx, err := RequireAppName(ctx)

	if errors.Is(err, ErrRequireAppName) {
		return appconfig.WithName(ctx, ""), nil
	}

	return localCtx, err
}

// LoadAppNameIfPresentNoFlag is like LoadAppNameIfPresent, but it does not check for the --app flag.
func LoadAppNameIfPresentNoFlag(ctx context.Context) (context.Context, error) {
	localCtx, err := RequireAppNameNoFlag(ctx)

	if errors.Is(err, ErrRequireAppName) {
		return appconfig.WithName(ctx, ""), nil
	}

	return localCtx, err
}

func ChangeWorkingDirectoryToFirstArgIfPresent(ctx context.Context) (context.Context, error) {
	wd := flag.FirstArg(ctx)
	if wd == "" {
		return ctx, nil
	}
	return ChangeWorkingDirectory(ctx, wd)
}

func ChangeWorkingDirectory(ctx context.Context, wd string) (context.Context, error) {
	if !filepath.IsAbs(wd) {
		p, err := filepath.Abs(wd)
		if err != nil {
			return nil, fmt.Errorf("failed converting %s to an absolute path: %w", wd, err)
		}
		wd = p
	}

	if err := os.Chdir(wd); err != nil {
		return nil, fmt.Errorf("failed changing working directory: %w", err)
	}

	return state.WithWorkingDirectory(ctx, wd), nil
}
