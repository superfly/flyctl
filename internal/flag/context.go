package flag

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/command_context"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flag/flagnames"
)

// NewContext derives a context that carries fs from ctx.
func NewContext(ctx context.Context, fs *pflag.FlagSet) context.Context {
	return flagctx.NewContext(ctx, fs)
}

// FromContext returns the FlagSet ctx carries. It panics in case ctx carries
// no FlagSet.
func FromContext(ctx context.Context) *pflag.FlagSet {
	return flagctx.FromContext(ctx)
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

func EnvNameFromCmd(cmd *cobra.Command) string {
	if cmd.Parent() != nil {
		varname := EnvNameFromCmd(cmd.Parent()) + "_" + cmd.Name()
		return strings.ToUpper(varname)
	} else {
		return strings.ToUpper(cmd.Name())
	}

}

func FromEnv(ctx context.Context, name string) string {
	cmd := command_context.FromContext(ctx)
	value := ""
	for cmd != nil {
		var_name := EnvNameFromCmd(cmd) + "_" + strings.ToUpper(name)
		var_name = strings.ReplaceAll(var_name, "-", "_")
		value = env.First(var_name)
		if value == "" {
			cmd = cmd.Parent()
		} else {
			return value
		}
	}
	return value
}

// GetString returns the value of the named string flag ctx carries.
func GetString(ctx context.Context, name string) string {
	if v, err := FromContext(ctx).GetString(name); err == nil && v != "" {
		return v
	} else if v := FromEnv(ctx, name); v != "" {
		return v
	} else {
		return ""
	}
}

// SetString sets the value of the named string flag ctx carries.
func SetString(ctx context.Context, name, value string) error {
	return FromContext(ctx).Set(name, value)
}

// GetInt returns the value of the named int flag ctx carries. It panics
// in case ctx carries no flags or in case the named flag isn't an int one.
func GetInt(ctx context.Context, name string) int {
	if v, err := FromContext(ctx).GetInt(name); err == nil {
		return v
	} else if v := FromEnv(ctx, name); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		} else {
			panic(err)
		}
	} else {
		panic(err)
	}
}

// GetFloat64 returns the value of the named int flag ctx carries. It panics
// in case ctx carries no flags or in case the named flag isn't a float64 one.
func GetFloat64(ctx context.Context, name string) float64 {
	if v, err := FromContext(ctx).GetFloat64(name); err == nil {
		return v
	} else if v := FromEnv(ctx, name); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		} else {
			panic(err)
		}
	} else {
		panic(err)
	}
}

// GetStringArray returns the values of the named string flag ctx carries.
// Preserves commas (unlike the following `GetStringSlice`): in `--flag x,y` the value is string[]{`x,y`}.
// This is useful to pass key-value pairs like environment variables or build arguments.
func GetStringArray(ctx context.Context, name string) []string {
	if v, err := FromContext(ctx).GetStringArray(name); err == nil {
		return v
	} else if v := FromEnv(ctx, name); v != "" {
		return []string{v}
	} else {
		return []string{}
	}
}

// GetStringSlice returns the values of the named string flag ctx carries.
// Can be comma separated or passed "by repeated flags": `--flag x,y` is equivalent to `--flag x --flag y`.
func GetStringSlice(ctx context.Context, name string) []string {
	if v, err := FromContext(ctx).GetStringSlice(name); err == nil {
		return v
	} else if v := FromEnv(ctx, name); v != "" {
		return strings.Split(v, ",")
	} else {
		return []string{}
	}
}

// GetStringSlice returns the values of the named string flag ctx carries. Can
// be comma separated or passed "by repeated flags": `--flag x,y` is equivalent
// to `--flag x --flag y`. Strings are trimmed of extra whitespace and empty
// strings are removed.
func GetNonEmptyStringSlice(ctx context.Context, name string) []string {
	if v, err := FromContext(ctx).GetStringSlice(name); err != nil {
		return []string{}
	} else {
		for i := range v {
			v[i] = strings.TrimSpace(v[i])
		}
		return slices.DeleteFunc(v, func(s string) bool { return s == "" })
	}
}

// GetDuration returns the value of the named duration flag ctx carries.
func GetDuration(ctx context.Context, name string) time.Duration {
	if v, err := FromContext(ctx).GetDuration(name); err == nil {
		return v
	} else if v := FromEnv(ctx, name); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		} else {
			return 0
		}
	} else {
		return 0
	}
}

// GetBool returns the value of the named boolean flag ctx carries.
func GetBool(ctx context.Context, name string) bool {
	isSpecified := IsSpecified(ctx, name)
	if v, err := FromContext(ctx).GetBool(name); err == nil && isSpecified {
		return v
	} else if v := FromEnv(ctx, name); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return false
}

// IsSpecified returns whether a flag has been specified at all or not.
// This is useful, for example, when differentiating between 0/"" and unspecified.
func IsSpecified(ctx context.Context, name string) bool {
	flag := FromContext(ctx).Lookup(name)
	return flag != nil && flag.Changed
}

// GetOrg is shorthand for GetString(ctx, Org).
func GetOrg(ctx context.Context) string {
	org := GetString(ctx, flagnames.Org)
	return org
}

// GetMPGClusterID is shorthand for GetString(ctx, "cluster").
func GetMPGClusterID(ctx context.Context) string {
	return GetString(ctx, flagnames.MPGClusterID)
}

// GetRegion is shorthand for GetString(ctx, Region).
func GetRegion(ctx context.Context) string {
	return GetString(ctx, flagnames.Region)
}

// GetYes is shorthand for GetBool(ctx, Yes).
func GetYes(ctx context.Context) bool {
	return GetBool(ctx, flagnames.Yes)
}

// GetApp is shorthand for GetString(ctx, App).
func GetApp(ctx context.Context) string {
	return GetString(ctx, flagnames.App)
}

// GetAppConfigFilePath is shorthand for GetString(ctx, AppConfigFilePath).
func GetAppConfigFilePath(ctx context.Context) string {
	if path, err := FromContext(ctx).GetString(flagnames.AppConfigFilePath); err != nil {
		return ""
	} else {
		return path
	}
}

// GetBindAddr is shorthand for GetString(ctx, BindAddr).
func GetBindAddr(ctx context.Context) string {
	return GetString(ctx, flagnames.BindAddr)
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

func GetProcessGroup(ctx context.Context) string {
	return GetString(ctx, flagnames.ProcessGroup)
}

func GetBuildkitAddr(ctx context.Context) string {
	addr := GetString(ctx, "buildkit-addr")
	if addr == "" {
		addr = env.First("BUILDKIT_ADDR")
	}
	return addr
}

func GetBuildkitImage(ctx context.Context) string {
	addr := GetString(ctx, "buildkit-image")
	if addr == "" {
		addr = env.First("BUILDKIT_IMAGE")
	}
	return addr
}
