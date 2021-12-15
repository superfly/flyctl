package flag

import (
	"context"

	"github.com/spf13/pflag"
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

// GetString returns the value of the named string flag ctx carries. It panics
// in case ctx carries no flags or in case the named flag isn't a string one.
func GetString(ctx context.Context, name string) string {
	if v, err := FromContext(ctx).GetString(name); err != nil {
		panic(err)
	} else {
		return v
	}
}

// GetBool returns the value of the named boolean flag ctx carries. It panics
// in case ctx carries no flags or in case the named flag isn't a boolean one.
func GetBool(ctx context.Context, name string) bool {
	if v, err := FromContext(ctx).GetBool(name); err != nil {
		panic(err)
	} else {
		return v
	}
}

// GetOrg is shorthand for GetString(ctx, OrgName).
func GetOrg(ctx context.Context) string {
	return GetString(ctx, OrgName)
}

// GetOrg is shorthand for GetBool(ctx, YesName).
func GetYes(ctx context.Context) bool {
	return GetBool(ctx, YesName)
}
