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

	track, err := trackFromRef(ref)
	if err != nil {
		return err
	}
	output.Track = track

	previousVersion, err := latestVersion(track)
	if err != nil {
		return err
	}
	if previousVersion != nil {
		str := previousVersion.String()
		output.PreviousVersion = &str
	}

	if previousVersion == nil {
		output.NextVersion = version.New(commitTime, track, 1).String()
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.Encode(output)

	return nil
}

// func newLatestVersionCmd() *cobra.Command {
// 	cmd := &cobra.Command{
// 		Use:   "latest",
// 		Short: "Prints the latest version for the current track",
// 		RunE: func(cmd *cobra.Command, args []string) error {
// 			if err := refreshTags(); err != nil {
// 				return err
// 			}
// 			cmd.PrintErrln("refreshed tags")

// 			ref, err := gitRef()
// 			if err != nil {
// 				return err
// 			}
// 			cmd.PrintErrln("ref:", ref)

// 			time, err := gitCommitTime(ref)
// 			if err != nil {
// 				return err
// 			}
// 			cmd.PrintErrln("commit time:", time)

// 			track, err := trackFromRef(ref)
// 			if err != nil {
// 				return err
// 			}
// 			cmd.PrintErrln("track:", track)

// 			currentVersion, err := latestVersion(track)
// 			if err != nil {
// 				return err
// 			}
// 			cmd.PrintErrln("current version:", currentVersion)

// 			cmd.Print(currentVersion)

// 			return nil
// 		},
// 	}

// 	return cmd
// }

type output struct {
	Track           string  `json:"track"`
	PreviousVersion *string `json:"previousVersion"`
	NextVersion     string  `json:"nextVersion"`
	Ref             string  `json:"ref"`
	CommitTime      string  `json:"commitTime"`
}

// func newNextVersionCmd() *cobra.Command {
// 	var quiet bool

// 	cmd := &cobra.Command{
// 		Use:   "next",
// 		Short: "Prints the next version for the current track",
// 	}
// 	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "only print the version number")

// 	cmd.RunE = func(cmd *cobra.Command, args []string) error {
// 		if err := refreshTags(); err != nil {
// 			return err
// 		}
// 		if !quiet {
// 			cmd.PrintErrln("refreshed tags")
// 		}

// 		ref, err := gitRef()
// 		if err != nil {
// 			return err
// 		}
// 		if !quiet {
// 			cmd.PrintErrln("ref:", ref)
// 		}

// 		time, err := gitCommitTime(ref)
// 		if err != nil {
// 			return err
// 		}
// 		if !quiet {
// 			cmd.PrintErrln("commit time:", time)
// 		}

// 		track, err := trackFromRef(ref)
// 		if err != nil {
// 			return err
// 		}
// 		cmd.PrintErrln("track:", track)

// 		currentVersion, err := latestVersion(track)
// 		if err != nil {
// 			return err
// 		}
// 		cmd.PrintErrln("current version:", currentVersion)

// 		buildNumber, err := nextBuildNumber(track, time)
// 		if err != nil {
// 			return err
// 		}
// 		cmd.PrintErrln("build number:", buildNumber)

// 		nextVersion := version.Version{
// 			Major: time.Year(),
// 			Minor: int(time.Month()),
// 			Patch: time.Day(),
// 			Track: track,
// 			Build: buildNumber,
// 		}

// 		cmd.Print(nextVersion)

// 		return nil
// 	}

// 	return cmd
// }
