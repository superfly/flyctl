package preparers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/httptracing"
	"github.com/superfly/flyctl/internal/instrument"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
)

type Preparer func(context.Context) (context.Context, error)

func LoadConfig(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)

	cfg := config.New()

	// Apply config from the config file, if it exists
	path := filepath.Join(state.ConfigDirectory(ctx), config.FileName)
	if err := cfg.ApplyFile(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	// Apply config from the environment, overriding anything from the file
	cfg.ApplyEnv()

	// Finally, apply command line options, overriding any previous setting
	cfg.ApplyFlags(flagctx.FromContext(ctx))

	logger.Debug("config initialized.")

	return config.NewContext(ctx, cfg), nil
}

func InitClient(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)
	cfg := config.FromContext(ctx)

	// TODO: refactor so that api package does NOT depend on global state
	api.SetBaseURL(cfg.APIBaseURL)
	api.SetErrorLog(cfg.LogGQLErrors)
	api.SetInstrumenter(instrument.ApiAdapter)
	api.SetTransport(httptracing.NewTransport(http.DefaultTransport))

	c := client.FromToken(cfg.AccessToken)
	logger.Debug("client initialized.")

	return client.NewContext(ctx, c), nil
}

func DetermineConfigDir(ctx context.Context) (context.Context, error) {
	dir := filepath.Join(state.UserHomeDirectory(ctx), ".fly")

	logger.FromContext(ctx).
		Debugf("determined config directory: %q", dir)

	return state.WithConfigDirectory(ctx, dir), nil
}

func DetermineUserHomeDir(ctx context.Context) (context.Context, error) {
	wd, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed determining user home directory: %w", err)
	}

	logger.FromContext(ctx).
		Debugf("determined user home directory: %q", wd)

	return state.WithUserHomeDirectory(ctx, wd), nil
}

// ApplyAliases consolidates flags with aliases into a single source-of-truth flag.
// After calling this, the main flags will have their values set as follows:
//   - If the main flag was already set, it will keep its value.
//   - If it was not set, but an alias was, it will take the value of the first specified alias flag.
//     This will set flag.Changed to true, as if it were specified manually.
//   - If none of the flags were set, the main flag will remain its default value.
func ApplyAliases(ctx context.Context) (context.Context, error) {

	var (
		invalidFlagNames []string
		invalidTypes     []string

		flags = flagctx.FromContext(ctx)
	)
	flags.VisitAll(func(f *pflag.Flag) {
		aliases, ok := f.Annotations["flyctl_alias"]
		if !ok {
			return
		}

		name := f.Name
		gotValue := false
		origFlag := flags.Lookup(name)

		if origFlag == nil {
			invalidFlagNames = append(invalidFlagNames, name)
		} else {
			gotValue = origFlag.Changed
		}

		for _, alias := range aliases {
			aliasFlag := flags.Lookup(alias)
			if aliasFlag == nil {
				invalidFlagNames = append(invalidFlagNames, alias)
				continue
			}
			if origFlag == nil {
				continue // nothing left to do here if we have no root flag
			}
			if aliasFlag.Value.Type() != origFlag.Value.Type() {
				invalidTypes = append(invalidTypes, fmt.Sprintf("%s (%s) and %s (%s)", name, origFlag.Value.Type(), alias, aliasFlag.Value.Type()))
			}
			if !gotValue && aliasFlag.Changed {
				err := origFlag.Value.Set(aliasFlag.Value.String())
				if err != nil {
					panic(err)
				}
				origFlag.Changed = true
			}
		}
	})

	var err error
	{
		var errorMessages []string
		if len(invalidFlagNames) > 0 {
			errorMessages = append(errorMessages, fmt.Sprintf("flags '%v' are not valid flags", invalidFlagNames))
		}
		if len(invalidTypes) > 0 {
			errorMessages = append(errorMessages, fmt.Sprintf("flags '%v' have different types", invalidTypes))
		}
		if len(errorMessages) > 1 {
			err = fmt.Errorf("multiple errors occured:\n > %s\n", strings.Join(errorMessages, "\n > "))
		} else if len(errorMessages) == 1 {
			err = fmt.Errorf("%s", errorMessages[0])
		}
	}
	return ctx, err
}
