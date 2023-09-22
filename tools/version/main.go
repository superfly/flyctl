package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/version"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "version",
		Short: "Tool for working with flyctl version numbers",
		RunE:  run,
	}

	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

func run(cmd *cobra.Command, args []string) error {
	if err := refreshTags(); err != nil {
		return err
	}

	output := output{}

	ref, err := gitRef()
	if err != nil {
		return err
	}
	output.Ref = ref

	commitTime, err := gitCommitTime(ref)
	if err != nil {
		return err
	}
	output.CommitTime = commitTime.Format(time.RFC3339)

	channel, err := channelFromRef(ref)
	if err != nil {
		return err
	}
	output.Channel = channel

	previousVersion, err := latestVersion(channel)
	if err != nil {
		return err
	}

	if previousVersion != nil {
		str := previousVersion.String()
		output.PreviousVersion = &str
		output.NextVersion = previousVersion.Increment(commitTime).String()
	} else {
		output.NextVersion = version.New(commitTime, channel, 1).String()
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.Encode(output)

	return nil
}

type output struct {
	Channel         string  `json:"channel"`
	PreviousVersion *string `json:"previousVersion"`
	NextVersion     string  `json:"nextVersion"`
	Ref             string  `json:"ref"`
	CommitTime      string  `json:"commitTime"`
}
