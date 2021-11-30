package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flyerr"
)

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

	rootCmd.AddCommand(
		newAuthCommand(client),
		newBuildsCommand(client),
		newCurlCommand(client),
		newCertificatesCommand(client),
		newConfigCommand(client),
		newDashboardCommand(client),
		newDeployCommand(client),
		newDocsCommand(client),
		newHistoryCommand(client),
		newInfoCommand(client),
		newIPAddressesCommand(client),
		newListCommand(client),
		newLogsCommand(client),
		newMonitorCommand(client),
		newOpenCommand(client),
		newPlatformCommand(client),
		newRegionsCommand(client),
		newReleasesCommand(client),
		newScaleCommand(client),
		newAutoscaleCommand(client),
		newSecretsCommand(client),
		newStatusCommand(client),
		newDNSCommand(client),
		newDomainsCommand(client),
		newImageCommand(client),
		newOrgsCommand(client),
		newVolumesCommand(client),
		newWireGuardCommand(client),
		newSSHCommand(client),
		newAgentCommand(client),
		newChecksCommand(client),
		newPostgresCommand(client),
		newVMCommand(client),
		newLaunchCommand(client),
		newMachineCommand(client),
		newProxyCommand(client),
		newDigCommand(client),
		newTurbokuCommand(client),
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
