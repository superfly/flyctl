package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
)

var ErrAbort = errors.New("abort")

var rootCmd = &Command{
	Command: &cobra.Command{
		Use:   "flyctl",
		Short: "The Fly CLI",
		Long:  `flyctl is a command line interface for the Fly.io platform`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
		},
	},
}

func GetRootCommand() *cobra.Command {
	return rootCmd.Command
}
func Execute() {
	defer flyctl.BackgroundTaskWG.Wait()

	err := rootCmd.Execute()
	checkErr(err)
}

func init() {
	rootCmd.PersistentFlags().StringP("access-token", "t", "", "Fly API Access Token")
	viper.BindPFlag(flyctl.ConfigAPIToken, rootCmd.PersistentFlags().Lookup("access-token"))

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	viper.BindPFlag(flyctl.ConfigVerboseOutput, rootCmd.PersistentFlags().Lookup("verbose"))

	rootCmd.AddCommand(
		newAuthCommand(),
		newAppStatusCommand(),
		newAppListCommand(),
		newAppReleasesListCommand(),
		newAppLogsCommand(),
		newAppSecretsCommand(),
		newVersionCommand(),
		newDeployCommand(),
		newAppInfoCommand(),
		newBuildsCommand(),
		newAppHistoryCommand(),
		newCertificatesCommand(),
		newDocsCommand(),
		newIPAddressesCommand(),
		newConfigCommand(),
	)

	initConfig()
}

func initConfig() {
	flyctl.InitConfig()
	flyctl.CheckForUpdate()
}

func checkErr(err error) {
	if err == nil {
		return
	}

	if err != ErrAbort {
		fmt.Println(aurora.Red("Error"), err)
	}

	safeExit()
}

func safeExit() {
	flyctl.BackgroundTaskWG.Wait()

	os.Exit(1)
}
