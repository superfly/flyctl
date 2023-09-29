package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/tools/distribute/flypkgs"
)

var (
	apiEndpoint = envOrDefault("FLYPKGS_API_ENDPOINT", "https://flyio-pkgs.fly.dev/api")
	apiToken    = mustEnv("FLYPKGS_API_TOKEN")
)

func main() {
	rootCmd := &cobra.Command{
		Use:          "distribute",
		Short:        "Distribute releases to pkgs.fly.io",
		SilenceUsage: true,
	}

	uploadCmd := &cobra.Command{
		Use:          "upload <path>",
		Short:        "upload a release from <path>",
		SilenceUsage: true,
		RunE:         runUpload,
	}
	uploadCmd.Args = cobra.ExactArgs(1)

	publishCmd := &cobra.Command{
		Use:          "publish <version>",
		Short:        "publish a previously uploaded version",
		SilenceUsage: true,
		RunE:         runPublish,
	}
	publishCmd.Args = cobra.ExactArgs(1)

	rootCmd.AddCommand(uploadCmd, publishCmd)

	if err := rootCmd.Execute(); err != nil {
		// fmt.Println(err)
		os.Exit(1)
	}
}

func runUpload(cmd *cobra.Command, args []string) error {
	distDir := args[0]

	ctx := cmd.Context()

	client := flypkgs.NewClient(apiEndpoint, apiToken)
	if err := checkExistingRelease(ctx, client, distDir); err != nil {
		return err
	}

	os.MkdirAll("./tmp", 0755)

	outFile, err := os.CreateTemp("./tmp", "release-*.tar.gz")
	if err != nil {
		return err
	}
	defer outFile.Close()

	fmt.Println("creating release archive")

	r, err := createReleaseArchive(distDir, outFile)
	if err != nil {
		return err
	}

	if _, err := outFile.Seek(0, 0); err != nil {
		return err
	}

	fmt.Println("Uploading release archive")

	release, err := client.UploadRelease(ctx, r.Version, outFile)
	if err != nil {
		return err
	}
	fmt.Println("Created release:")
	fmt.Printf("\tVersion: %s\n", release.Version)
	fmt.Printf("\tChannel: %s (status:%s, stable:%t)\n", release.Channel.Name, release.Channel.Status, release.Channel.Stable)
	fmt.Printf("\tStatus: %s\n", release.Status)
	fmt.Printf("\tGit Commit: %s\n", release.GitCommit)
	fmt.Printf("\tGit Branch: %s\n", release.GitBranch)
	fmt.Printf("\tGit Tag: %s\n", release.GitTag)

	return nil
}

func runPublish(cmd *cobra.Command, args []string) error {
	version := args[0]

	client := flypkgs.NewClient(apiEndpoint, apiToken)
	release, err := client.PublishRelease(cmd.Context(), version)
	if err != nil {
		return err
	}

	fmt.Println("Published release", release.Version)
	return nil
}

func checkExistingRelease(ctx context.Context, client *flypkgs.Client, distDir string) error {
	buildInfo, err := loadJSONFile[buildInfo](filepath.Join(distDir, "metadata.json"))
	if err != nil {
		return errors.Wrap(err, "loading build info")
	}
	version := buildInfo.Version

	release, err := client.GetReleaseByVersion(ctx, version)
	if flypkgs.IsNotFoundErr(err) {
		return nil
	}
	if err == nil && release != nil {
		return fmt.Errorf("release %s already exists", version)
	}
	return err
}

func envOrDefault(varName string, defaultValue string) string {
	if v := os.Getenv(varName); v != "" {
		return v
	}
	return defaultValue
}

func mustEnv(varName string) string {
	if v := os.Getenv(varName); v != "" {
		return v
	}
	panic(fmt.Sprintf("missing required environment variable %s", varName))
}
