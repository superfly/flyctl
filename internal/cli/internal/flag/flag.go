// Package flag implements flag-related functionality.
package flag

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// AccessToken denotes the name of the access token flag.
	AccessToken = "access-token"

	// Verbose denotes the name of the verbose flag.
	Verbose = "verbose"

	// JSON denotes the name of the verbose flag.
	JSON = "json"
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
}

// Int wraps the set of int flags.
type Int struct {
	Name        string
	Shorthand   string
	Description string
	Default     int
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
