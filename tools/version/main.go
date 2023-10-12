package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/tools/version/relmeta"
)

// TODO[md]: remove this when we're done with the semver to calver migration
const stableChannelStillOnSemver = true

var (
	gitDir string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "version",
		Short: "Tool for working with flyctl version numbers",
	}
	rootCmd.PersistentFlags().StringVar(&gitDir, "git-dir", "", "path to git directory. defaults to current directory.")

	showCmd := &cobra.Command{
		Use:          "show",
		Short:        "Show version information as a JSON object",
		SilenceUsage: true,
		RunE:         runShow,
	}

	nextCmd := &cobra.Command{
		Use:          "next",
		Short:        "show the next version number for the current channel",
		SilenceUsage: true,
		RunE:         runNext,
	}

	rootCmd.AddCommand(showCmd, nextCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

func runShow(cmd *cobra.Command, args []string) error {
	cmd.PrintErrln("Git dir:", gitDir)

	if err := relmeta.RefreshTags(gitDir); err != nil {
		return err
	}

	meta, err := relmeta.GenerateReleaseMeta(gitDir, stableChannelStillOnSemver)
	if err != nil {
		return err
	}

	if meta.Version == nil {
		return fmt.Errorf("no version tag for commit %s", meta.Commit)
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.Encode(meta)

	return nil
}

func runNext(cmd *cobra.Command, args []string) error {
	cmd.PrintErrln("Git dir:", gitDir)

	if err := relmeta.RefreshTags(gitDir); err != nil {
		return err
	}

	ver, err := relmeta.NextVersion(gitDir, stableChannelStillOnSemver)
	if err != nil {
		return err
	}

	fmt.Fprint(cmd.OutOrStdout(), ver.String())

	return nil
}
