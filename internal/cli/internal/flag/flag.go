// Package flag implements flag-related functionality.
package flag

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/config"
)

// Flag wraps the set of flags.
type Flag interface {
	addTo(*cobra.Command, *viper.Viper)
}

// Add adds flag to cmd, binding them on v should v not be nil.
func Add(cmd *cobra.Command, v *viper.Viper, flags ...Flag) {
	for _, flag := range flags {
		flag.addTo(cmd, v)
	}
}

// Bool wraps the set of boolean flags.
type Bool struct {
	Name        string
	Shorthand   string
	Description string
	Default     bool
	ConfName    string
	EnvName     string
	Hidden      bool
}

func (b Bool) addTo(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()

	if b.Shorthand != "" {
		_ = flags.BoolP(b.Name, b.Shorthand, b.Default, b.Description)
	} else {
		_ = flags.Bool(b.Name, b.Default, b.Description)
	}

	f := flags.Lookup(b.Name)
	f.Hidden = b.Hidden

	Bind(v, f, f.Name, b.ConfName, b.EnvName)
}

// String wraps the set of string flags.
type String struct {
	Name        string
	Shorthand   string
	Description string
	Default     string
	ConfName    string
	EnvName     string
	Hidden      bool
}

func (s String) addTo(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()

	if s.Shorthand != "" {
		_ = flags.StringP(s.Name, s.Shorthand, s.Default, s.Description)
	} else {
		_ = flags.String(s.Name, s.Default, s.Description)
	}

	f := flags.Lookup(s.Name)
	f.Hidden = s.Hidden

	Bind(v, f, f.Name, s.ConfName, s.EnvName)
}

// Int wraps the set of int flags.
type Int struct {
	Name        string
	Shorthand   string
	Description string
	Default     int
	ConfName    string
	EnvName     string
	Hidden      bool
}

func (i Int) addTo(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()

	if i.Shorthand != "" {
		_ = flags.IntP(i.Name, i.Shorthand, i.Default, i.Description)
	} else {
		_ = flags.Int(i.Name, i.Default, i.Description)
	}

	f := flags.Lookup(i.Name)
	f.Hidden = i.Hidden

	Bind(v, f, i.Name, i.ConfName, i.EnvName)
}

// StringSlice wraps the set of string slice flags.
type StringSlice struct {
	Name        string
	Shorthand   string
	Description string
	Default     []string
	ConfName    string
	EnvName     string
}

func (ss StringSlice) addTo(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()

	if ss.Shorthand != "" {
		_ = flags.StringSliceP(ss.Name, ss.Shorthand, ss.Default, ss.Description)
	} else {
		_ = flags.StringSlice(ss.Name, ss.Default, ss.Description)
	}

	Bind(v, flags.Lookup(ss.Name), ss.Name, ss.ConfName, ss.EnvName)
}

func fs(cmd *cobra.Command, persistent bool) *pflag.FlagSet {
	if persistent {
		return cmd.PersistentFlags()
	}

	return cmd.Flags()
}

func Bind(v *viper.Viper, flag *pflag.Flag, name, confName, envName string) {
	if confName != "" {
		if err := v.BindPFlag(confName, flag); err != nil {
			panic(err)
		}
	}

	if envName != "" {
		if err := v.BindEnv(name, envName); err != nil {
			panic(err)
		}
	}
}

func namespace(cmd *cobra.Command) string {
	parentName := flyctl.NSRoot
	if cmd.Parent() != nil {
		parentName = cmd.Parent().Name()
	}

	return fmt.Sprintf("%s.%s", parentName, cmd.Name())
}

// Org returns an org string flag.
func Org() String {
	return String{
		Name:        "org",
		Description: `The organization that will own the app`,
	}
}

// Yes returns a yes bool flag.
func Yes() Bool {
	return Bool{
		Name:        "yes",
		Shorthand:   "y",
		Description: "Accept all confirmations",
	}
}

// GetAccessToken returns the value of the access token flag the FlagSet of ctx
// carries. It panics in case ctx carries no FlagSet.
func GetAccessToken(ctx context.Context) string {
	v, _ := GetString(ctx, config.AccessTokenKey)

	return v
}

// GetJSONOutput returns the value of the JSON output flag the FlagSet of ctx
// carries. It panics in case ctx carries no FlagSet.
func GetJSONOutput(ctx context.Context) bool {
	v, _ := GetBool(ctx, config.JSONOutputKey)

	return v
}

// GetVerboseOutput returns the value of the verbose output flag the FlagSet of
// ctx carries. It panics in case ctx carries no FlagSet.
func GetVerboseOutput(ctx context.Context) bool {
	v, _ := GetBool(ctx, config.VerboseOutputKey)

	return v
}

func coalesce(tokens ...string) (token string) {
	for _, t := range tokens {
		if t != "" {
			token = t

			break
		}
	}

	return
}
