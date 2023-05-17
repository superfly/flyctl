package flag

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	"github.com/spf13/pflag"
	"golang.org/x/exp/slices"
)

type contextKey struct{}

// NewContext derives a context that carries fs from ctx.
func NewContext(ctx context.Context, fs *pflag.FlagSet) context.Context {
	return context.WithValue(ctx, contextKey{}, fs)
}

// FromContext returns the FlagSet ctx carries. It panics in case ctx carries
// no FlagSet.
func FromContext(ctx context.Context) *pflag.FlagSet {
	return ctx.Value(contextKey{}).(*pflag.FlagSet)
}

// Args is shorthand for FromContext(ctx).Args().
func Args(ctx context.Context) []string {
	return FromContext(ctx).Args()
}

// FirstArg returns the first arg ctx carries or an empty string in case ctx
// carries an empty argument set. It panics in case ctx carries no FlagSet.
func FirstArg(ctx context.Context) string {
	if args := Args(ctx); len(args) > 0 {
		return args[0]
	}

	return ""
}

// GetString returns the value of the named string flag ctx carries.
func GetString(ctx context.Context, name string) string {
	if v, err := FromContext(ctx).GetString(name); err != nil {
		return ""
	} else {
		return v
	}
}

// SetString sets the value of the named string flag ctx carries.
func SetString(ctx context.Context, name, value string) error {
	return FromContext(ctx).Set(name, value)
}

// GetInt returns the value of the named int flag ctx carries. It panics
// in case ctx carries no flags or in case the named flag isn't an int one.
func GetInt(ctx context.Context, name string) int {
	if v, err := FromContext(ctx).GetInt(name); err != nil {
		panic(err)
	} else {
		return v
	}
}

// GetFirstInt returns the value of the first matching int flag ctx carries.
// It panics in case ctx carries no flags, or if any non-int flags are specified.
// If no values are specified, the *first* default value is returned, if any exist.
func GetFirstInt(ctx context.Context, name string, aliases ...string) int {
	var (
		flags        = FromContext(ctx)
		lastDefault  = 0
		anyWereValid = false
	)

	// Written this awkward way to enforce there being at least one name,
	// and to get IDE hints about how to use this function.
	names := append([]string{name}, aliases...)

	// Validate that all flags are int flags.
	invalidFlags := lo.Filter(names, func(name string, _ int) bool {
		info := flags.Lookup(name)
		return info == nil || info.Value.Type() != "int"
	})
	if len(invalidFlags) > 0 {
		panic(fmt.Errorf("flags '%v' are not int flags", invalidFlags))
	}

	// Get the first user-specified value, or the first default value.
	for _, name := range names {
		info := flags.Lookup(name)
		if info == nil {
			continue
		}
		anyWereValid = true
		value := GetInt(ctx, name)
		if info.Changed {
			return value
		} else {
			if lastDefault == 0 {
				lastDefault = value
			}
		}
	}
	if anyWereValid {
		return lastDefault
	}
	panic(fmt.Errorf("no int flag specified: %v", names))
}

// GetStringArray returns the values of the named string flag ctx carries.
// Preserves commas (unlike the following `GetStringSlice`): in `--flag x,y` the value is string[]{`x,y`}.
// This is useful to pass key-value pairs like environment variables or build arguments.
func GetStringArray(ctx context.Context, name string) []string {
	if v, err := FromContext(ctx).GetStringArray(name); err != nil {
		return []string{}
	} else {
		return v
	}
}

// GetStringSlice returns the values of the named string flag ctx carries.
// Can be comma separated or passed "by repeated flags": `--flag x,y` is equivalent to `--flag x --flag y`.
func GetStringSlice(ctx context.Context, name string) []string {
	if v, err := FromContext(ctx).GetStringSlice(name); err != nil {
		return []string{}
	} else {
		return v
	}
}

// GetDuration returns the value of the named duration flag ctx carries.
func GetDuration(ctx context.Context, name string) time.Duration {
	if v, err := FromContext(ctx).GetDuration(name); err != nil {
		return 0
	} else {
		return v
	}
}

// GetBool returns the value of the named boolean flag ctx carries.
func GetBool(ctx context.Context, name string) bool {
	if v, err := FromContext(ctx).GetBool(name); err != nil {
		return false
	} else {
		return v
	}
}

// IsSpecified returns true if any of the provided flags have been specified.
// This is useful, for example, when differentiating between 0/"" and unspecified.
func IsSpecified(ctx context.Context, names ...string) bool {
	for _, name := range names {
		flag := FromContext(ctx).Lookup(name)
		if flag != nil && flag.Changed {
			return true
		}
	}
	return false
}

// GetOrg is shorthand for GetString(ctx, OrgName).
func GetOrg(ctx context.Context) string {
	return GetString(ctx, OrgName)
}

// GetRegion is shorthand for GetString(ctx, RegionName).
func GetRegion(ctx context.Context) string {
	return GetString(ctx, RegionName)
}

// GetYes is shorthand for GetBool(ctx, YesName).
func GetYes(ctx context.Context) bool {
	return GetBool(ctx, YesName)
}

// GetApp is shorthand for GetString(ctx, AppName).
func GetApp(ctx context.Context) string {
	return GetString(ctx, AppName)
}

// GetAppConfigFilePath is shorthand for GetString(ctx, AppConfigFilePathName).
func GetAppConfigFilePath(ctx context.Context) string {
	if path, err := FromContext(ctx).GetString(AppConfigFilePathName); err != nil {
		return ""
	} else {
		return path
	}
}

// GetFlagsName returns the name of flags that have been set except unwanted flags.
func GetFlagsName(ctx context.Context, ignoreFlags []string) []string {
	flagsName := []string{}

	FromContext(ctx).Visit(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}

		if !slices.Contains(ignoreFlags, f.Name) {
			flagsName = append(flagsName, f.Name)
		}
	})

	return flagsName
}
