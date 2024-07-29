package preparers

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/instrument"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Preparers are split between here and `command/command.go` because
// tab-completion needs to run *some* of them, and importing the command package from there
// would create a circular dependency. Likewise, if *all* the preparers were in this module,
// that would also cause a circular dependency.
// I don't like this, but it's shippable until someone else fixes it

type Preparer func(context.Context) (context.Context, error)

func LoadConfig(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)

	cfg, err := config.Load(ctx, filepath.Join(state.ConfigDirectory(ctx), config.FileName))
	if err != nil {
		return nil, err
	}

	logger.Debug("config initialized.")

	return config.NewContext(ctx, cfg), nil
}

func InitClient(ctx context.Context) (context.Context, error) {
	logger := logger.FromContext(ctx)
	cfg := config.FromContext(ctx)

	// TODO: refactor so that api package does NOT depend on global state
	fly.SetBaseURL(cfg.APIBaseURL)
	fly.SetErrorLog(cfg.LogGQLErrors)
	fly.SetInstrumenter(instrument.ApiAdapter)
	fly.SetTransport(otelhttp.NewTransport(http.DefaultTransport))

	if flyutil.ClientFromContext(ctx) == nil {
		client := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{Tokens: cfg.Tokens})
		logger.Debug("client initialized.")
		ctx = flyutil.NewContextWithClient(ctx, client)
	}

	return ctx, nil
}

func DetermineConfigDir(ctx context.Context) (context.Context, error) {
	dir, err := helpers.GetConfigDirectory()
	if err != nil {
		return ctx, err
	}

	logger.FromContext(ctx).
		Debugf("determined config directory: %q", dir)

	return state.WithConfigDirectory(ctx, dir), nil
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
