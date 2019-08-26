package cmd

import (
	"fmt"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
)

var rootCmd = &Command{
	Command: &cobra.Command{
		Use:  "flyctl",
		Long: `flycyl is a command line interface for the Fly.io platform`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
		},
	},
}

func Execute() {
	defer flyctl.BackgroundTaskWG.Wait()

	err := rootCmd.Execute()
	checkErr(err)
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringP("access-token", "t", "", "Fly API Access Token")
	viper.BindPFlag(flyctl.ConfigAPIAccessToken, rootCmd.PersistentFlags().Lookup("access-token"))
	viper.BindEnv(flyctl.ConfigAPIAccessToken, "FLY_ACCESS_TOKEN")

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	viper.BindPFlag(flyctl.ConfigVerboseOutput, rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindEnv(flyctl.ConfigVerboseOutput, "VERBOSE")

	rootCmd.AddCommand(
		newAuthCommand(),
		newAppStatusCommand(),
		newAppListCommand(),
		newAppReleasesListCommand(),
		newAppLogsCommand(),
		newAppSecretsCommand(),
		newVersionCommand(),
		newAppCreateCommand(),
		newDeployCommand(),
		newAppInfoCommand(),
		newBuildsCommand(),
		newDatabasesCommand(),
		newAppHistoryCommand(),
	)
}

func initConfig() {
	flyctl.InitConfig()
	flyctl.CheckForUpdate()
}

func checkErr(err error) {
	if err == nil {
		return
	}

	fmt.Println(aurora.Red("Error"), err)

	safeExit()
}

func safeExit() {
	flyctl.BackgroundTaskWG.Wait()

	os.Exit(1)
}
