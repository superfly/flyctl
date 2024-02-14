package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/version"
	"github.com/superfly/flyctl/tools/distribute/bundle"
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
		Use:          "upload [path]",
		Short:        "upload a release from [path]",
		SilenceUsage: true,
		RunE:         runUpload,
	}
	uploadCmd.Args = cobra.MaximumNArgs(1)

	publishCmd := &cobra.Command{
		Use:          "publish <version>",
		Short:        "publish a previously uploaded version",
		SilenceUsage: true,
		RunE:         runPublish,
	}
	publishCmd.Args = cobra.ExactArgs(1)

	rootCmd.AddCommand(uploadCmd, publishCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runUpload(cmd *cobra.Command, args []string) error {
	distDir := "./dist"

	if len(args) > 0 {
		distDir = args[0]
	}

	distDir, err := filepath.Abs(distDir)
	if err != nil {
		return err
	}

	fmt.Println("Dist Dir:", distDir)

	meta, err := bundle.BuildMeta(distDir)
	if err != nil {
		return err
	}

	if err := meta.Validate(); err != nil {
		return err
	}

	fmt.Println("Release Meta")
	fmt.Println("  Version:", meta.Release.Version)
	fmt.Println("  Tag:", meta.Release.Tag)
	fmt.Println("  Commit:", meta.Release.Commit)
	fmt.Println("    Assets:")
	for _, asset := range meta.Assets {
		fmt.Println("    ", asset.Name)
		fmt.Println("    ", asset.AbsolutePath)
		fmt.Println("    ", asset.Path)
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", " ")
	enc.Encode(meta)

	ctx := cmd.Context()

	client := flypkgs.NewClient(apiEndpoint, apiToken)
	if err := checkExistingRelease(ctx, client, *meta.Release.Version); err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("Building release bundle")

	outFile, err := os.CreateTemp("", "release-*.tar.gz")
	if err != nil {
		return err
	}
	defer outFile.Close()
	defer os.RemoveAll(outFile.Name())

	bundle, err := bundle.CreateReleaseBundle(meta, outFile)
	if err != nil {
		return err
	}
	defer bundle.Close()

	if _, err := outFile.Seek(0, 0); err != nil {
		return err
	}

	fmt.Println("Uploading release bundle")

	release, err := client.UploadRelease(ctx, *meta.Release.Version, outFile)
	if err != nil {
		return err
	}
	fmt.Println("Created release:")
	fmt.Printf("  Version: %s\n", release.Version)
	fmt.Printf("  Status: %s\n", release.Status)
	fmt.Printf("  Channel: %s (status:%s, stable:%t)\n", release.Channel.Name, release.Channel.Status, release.Channel.Stable)

	return nil
}

func runPublish(cmd *cobra.Command, args []string) error {
	version, err := version.Parse(args[0])
	if err != nil {
		return err
	}

	client := flypkgs.NewClient(apiEndpoint, apiToken)
	release, err := client.PublishRelease(cmd.Context(), version)
	if err != nil {
		return err
	}

	fmt.Println("Published release", release.Version)
	return nil
}

func checkExistingRelease(ctx context.Context, client *flypkgs.Client, v version.Version) error {
	release, err := client.GetReleaseByVersion(ctx, v)
	if flypkgs.IsNotFoundErr(err) {
		return nil
	}
	if err == nil && release != nil {
		return fmt.Errorf("release %s already exists", v)
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
