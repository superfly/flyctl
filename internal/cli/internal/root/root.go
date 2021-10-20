// Package root implements the root command.
package root

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/superfly/flyctl/internal/cli/internal/apps"
	"github.com/superfly/flyctl/internal/cli/internal/cmd"
	"github.com/superfly/flyctl/internal/cli/internal/version"
	"github.com/superfly/flyctl/internal/config"
)

// New initializes and returns a reference to a new root command.
func New(v *viper.Viper) *cobra.Command {
	root := cmd.New("flyctl", nil)
	root.SilenceUsage = true
	root.SilenceErrors = true

	fs := root.PersistentFlags()

	_ = fs.StringP(config.AccessTokenKey, "t", "", "Fly API Access Token")
	bind(fs, v, config.AccessTokenKey, config.AccessTokenEnvKey)

	_ = fs.StringP(config.JSONOutputKey, "j", "", "JSON output")
	bind(fs, v, config.JSONOutputKey, config.JSONOutputEnvKey)

	_ = fs.StringP(config.VerboseOutputKey, "v", "", "Verbose output")
	bind(fs, v, config.VerboseOutputKey, config.VerboseOutputEnvKey)

	root.AddCommand(
		apps.New(),
		version.New(),
	)

	return root
}

func bind(fs *pflag.FlagSet, v *viper.Viper, name, envName string) {
	if err := v.BindPFlag(name, fs.Lookup(name)); err != nil {
		err = fmt.Errorf("error binding %q: %w", name, err)

		panic(err)
	}
}
