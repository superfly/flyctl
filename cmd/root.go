package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/flyerr"
)

// BUG(tqbf): this code is called by root.New() in internal/command/root/root.go; we're apparently
// halfway through a migration out of flyctl/cmd/ and into internal/command/, which I support, but
// this is obviously pretty confusing. I lost 8 minutes to figuring this out so you don't have to.
func NewRootCmd(client *client.Client) *cobra.Command {
	rootStrings := docstrings.Get("flyctl")
	rootCmd := &Command{
		Command: &cobra.Command{
			Use:   rootStrings.Usage,
			Short: rootStrings.Short,
			Long:  rootStrings.Long,
			PersistentPreRun: func(cmd *cobra.Command, args []string) {
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
			},
		},
	}

	rootCmd.PersistentFlags().StringP("access-token", "t", "", "Fly API Access Token")
	err := viper.BindPFlag(flyctl.ConfigAPIToken, rootCmd.PersistentFlags().Lookup("access-token"))
	checkErr(err)

	rootCmd.PersistentFlags().Bool("verbose", false, "verbose output")
	err = viper.BindPFlag(flyctl.ConfigVerboseOutput, rootCmd.PersistentFlags().Lookup("verbose"))
	checkErr(err)

	rootCmd.PersistentFlags().BoolP("json", "j", false, "json output")
	err = viper.BindPFlag(flyctl.ConfigJSONOutput, rootCmd.PersistentFlags().Lookup("json"))
	checkErr(err)

	rootCmd.PersistentFlags().String("builtinsfile", "", "Load builtins from named file")
	err = viper.BindPFlag(flyctl.ConfigBuiltinsfile, rootCmd.PersistentFlags().Lookup("builtinsfile"))
	checkErr(err)

	err = rootCmd.PersistentFlags().MarkHidden("builtinsfile")
	checkErr(err)

	rootCmd.SetHelpCommand(&cobra.Command{
		Use:    "no-help",
		Hidden: true,
	})

	rootCmd.AddCommand(
		newCertificatesCommand(client),
		newConfigCommand(client),
		newDashboardCommand(client),
		newInfoCommand(client),
		newListCommand(client),
		newRegionsCommand(client),
		newScaleCommand(client),
		newAutoscaleCommand(client),
		newDNSCommand(client),
		newDomainsCommand(client),
		newWireGuardCommand(client),
		newTurbokuCommand(client),
		newLaunchCommand(client),
	)

	return rootCmd.Command
}

func checkErr(err error) {
	if err == nil {
		return
	}

	if !isCancelledError(err) {
		fmt.Println(aurora.Red("Error"), err)
	}

	os.Exit(1)
}

func isCancelledError(err error) bool {
	if err == flyerr.ErrAbort {
		return true
	}

	if err == context.Canceled {
		return true
	}

	if merr, ok := err.(*multierror.Error); ok {
		if len(merr.Errors) == 1 && merr.Errors[0] == context.Canceled {
			return true
		}
	}

	return false
}
