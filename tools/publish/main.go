package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/tools/publish/flypkgs"
)

func main() {
	rootCmd := &cobra.Command{
		Use:          "publish <path>",
		Short:        "Publish a release to pkgs.fly.io",
		Long:         "Publish a release from <path> to pkgs.fly.io",
		SilenceUsage: true,
		RunE:         run,
	}

	rootCmd.Args = cobra.ExactArgs(1)

	if err := rootCmd.Execute(); err != nil {
		// fmt.Println(err)
		os.Exit(1)
	}
}

const FlyPkgsEndpoint = "https://flyio-pkgs.fly.dev/api"

type buildInfo struct {
	ProjectName string `json:"project_name"`
	Tag         string `json:"tag"`
	PreviousTag string `json:"previous_tag"`
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	Date        string `json:"date"`
}

type buildArtifact struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Goos         string `json:"goos"`
	Goarch       string `json:"goarch"`
	InternalType int    `json:"internal_type"`
	Type         string `json:"type"`
	Extra        struct {
		Binaries  []string `json:"Binaries"`
		Checksum  string   `json:"Checksum"`
		Format    string   `json:"Format"`
		ID        string   `json:"ID"`
		Replaces  string   `json:"Replaces"`
		WrappedIn string   `json:"WrappedIn"`
	} `json:"extra"`
}

func loadJSONFile[T any](path string) (T, error) {
	var data T

	file, err := os.Open(path)
	if err != nil {
		return data, err
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		return data, err
	}
	return data, nil
}

func run(cmd *cobra.Command, args []string) error {
	distDir := args[0]

	apiToken := os.Getenv("FLYPKGS_API_TOKEN")
	if apiToken == "" {
		return fmt.Errorf("FLYPKGS_API_TOKEN not set")
	}

	apiEndpoint := os.Getenv("FLYPKGS_API_ENDPOINT")
	if apiEndpoint == "" {
		apiEndpoint = FlyPkgsEndpoint
	}

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
