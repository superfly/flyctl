package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/dist/flypkgs"
	"golang.org/x/sync/errgroup"
)

func main() {
	rootCmd := &cobra.Command{
		Use:          "dist",
		Short:        "Distribute releases to pkgs.fly.io",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doit(cmd.Context())
		},
	}

	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

const FlyPkgsEndpoint = "http://localhost:4000/api"

// type release struct {
// 	ID        uint64 `json:"id"`
// 	Channel   string `json:"channel"`
// 	Version   string `json:"version"`
// 	GitCommit string `json:"git_commit"`
// 	GitBranch string `json:"git_branch"`
// 	GitRef    string `json:"git_ref"`
// 	Source    string `json:"source"`
// 	Status    string `json:"status"`
// }

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

func loadBuildInfo() (*buildInfo, error) {
	file, err := os.Open("dist/metadata.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info := &buildInfo{}
	err = json.NewDecoder(file).Decode(info)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func loadBuildArtifacts() ([]buildArtifact, error) {
	file, err := os.Open("dist/artifacts.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	artifacts := []buildArtifact{}
	err = json.NewDecoder(file).Decode(&artifacts)
	if err != nil {
		return nil, err
	}
	return artifacts, nil
}

func doit(ctx context.Context) error {
	buildInfo, err := loadBuildInfo()
	if err != nil {
		return errors.Wrap(err, "loading build info")
	}

	artifacts, err := loadBuildArtifacts()
	if err != nil {
		return errors.Wrap(err, "loading build artifacts")
	}

	fmt.Println("Distributing Build")
	fmt.Println("  Version:", buildInfo.Version)
	fmt.Println("  Tag:", buildInfo.Tag)
	fmt.Println("  Previous Tag:", buildInfo.PreviousTag)
	fmt.Println("  Commit:", buildInfo.Commit)
	fmt.Println("  Date:", buildInfo.Date)

	fmt.Println("  Artifacts:")
	for _, artifact := range artifacts {
		fmt.Printf("    %s: %s\n", artifact.Type, artifact.Path)
	}

	client := flypkgs.NewClient("apiKey")

	// var release *flypkgs.Release

	// release, err = client.GetReleaseByVersion(ctx, buildInfo.Version)
	// if flypkgs.IsNotFoundErr(err) {
	// 	fmt.Println("No existing release found, creating new release")

	reqIn := flypkgs.CreateReleaseInput{
		Channel:   "stable",
		Version:   buildInfo.Version,
		GitRef:    buildInfo.Tag,
		GitCommit: buildInfo.Commit,
		GitBranch: "master",
		Source:    "direct",
		// Status:    "waiting_for_assets",
	}

	release, err := client.CreateRelease(ctx, reqIn)
	if err != nil {
		if flypkgs.IsConflictError(err) {
			return errors.New("release already exists, cannot continue publishing the same version")
		} else {
			return errors.Wrap(err, "creating release")
		}
	}

	fmt.Println("	Done", release)

	// 	// fmt.Println("  Done", newRelease)
	// } else if err != nil {
	// 	return errors.Wrap(err, "getting existing release")
	// }
	// if release != nil {
	// 	fmt.Println("Doing it", release)
	// }

	// if existingRelease.Status != "waiting_for_assets" {
	// 	return errors.Errorf("existing release state is \"%s\", expected \"waiting_for_assets\"", existingRelease.Status)
	// }

	eg := new(errgroup.Group)
	// eg.SetLimit(1)

	for _, artifact := range artifacts {

		if !shouldCreateArtifact(artifact) {
			// fmt.Println("  Skipping")
			continue
		}

		fmt.Println("Uploading artifact:", artifact.Name)

		existing, err := client.GetAsset(ctx, release.Version, artifact.Goos, artifact.Goos)
		if flypkgs.IsNotFoundErr(err) {
			fmt.Println("  No existing asset found, creating new asset")
		} else if err != nil {
			return errors.Wrap(err, "getting existing asset")
		} else if existing != nil {
			fmt.Println("Existing:", existing)
			panic("stop")

		}

		path := artifact.Path
		input := flypkgs.CreateAssetInput{
			ReleaseID: release.ID,
			Name:      artifact.Name,
			Checksum:  artifact.Extra.Checksum,
			OS:        artifact.Goos,
			Arch:      artifact.Goarch,
		}

		eg.Go(func() error {
			fmt.Println("starting")
			defer fmt.Println("done")

			fmt.Println("a")
			file, err := os.Open(path)
			fmt.Println("b")
			if err != nil {
				fmt.Println("c")
				return errors.Wrap(err, "opening artifact")
			}
			fmt.Println("d")
			defer file.Close()

			asset, err := client.CreateAsset(ctx, input, file)
			fmt.Println("e")
			if err != nil {
				fmt.Println("f")
				fmt.Println("  Error creating asset:", err)
				return errors.Wrap(err, "creating asset")
			}
			fmt.Println("g")
			fmt.Println("  Created asset", asset.Name)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		fmt.Println("Error uploading artifacts:", err)
		return errors.Wrap(err, "uploading artifacts")
	}
	fmt.Println("All artifacts uploaded")

	fmt.Println("Relaese Done")

	return nil
}

// type createReleaseInput struct {
// 	Channel   string `json:"channel"`
// 	Version   string `json:"version"`
// 	GitCommit string `json:"git_commit"`
// 	GitBranch string `json:"git_branch"`
// 	GitRef    string `json:"git_ref"`
// 	Source    string `json:"source"`
// 	Status    string `json:"status"`
// }

// func getRelease(vNum string) (*release, error) {
// 	resp, err := http.Get(FlyPkgsEndpoint + "/releases/" + vNum)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode == http.StatusNotFound {
// 		return nil, nil
// 	}

// 	var release release
// 	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
// 		return nil, err
// 	}

// 	return &release, nil
// }

// func createRelease(input createReleaseInput) error {
// 	var body bytes.Buffer
// 	if err := json.NewEncoder(&body).Encode(input); err != nil {
// 		return err
// 	}

// 	// Create the HTTP request
// 	req, err := http.NewRequest("POST", FlyPkgsEndpoint+"/releases", &body)
// 	if err != nil {
// 		fmt.Println(err)
// 		return err
// 	}
// 	req.Header.Set("Content-Type", "application/json")

// 	// Send the request
// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		fmt.Println(err)
// 		return err
// 	}
// 	defer resp.Body.Close()

// 	// Read the response body
// 	respBody, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		fmt.Println(err)
// 		return err
// 	}

// 	fmt.Println(resp.StatusCode)

// 	// switch {
// 	// case resp.StatusCode >= 200 && resp.StatusCode < 300:
// 	// 	return nil
// 	// case resp.StatusCode == 422:
// 	fmt.Println(string(respBody))
// 	// }

// 	return nil
// }

func shouldCreateArtifact(artifact buildArtifact) bool {
	return artifact.Type == "Archive"
}

// func createArtifact(r io.Reader, version string, artifact buildArtifact) error {
// 	// Create a new multipart form
// 	body := &bytes.Buffer{}
// 	writer := multipart.NewWriter(body)

// 	if err := writer.WriteField("os", artifact.Goos); err != nil {
// 		return err
// 	}
// 	if err := writer.WriteField("arch", artifact.Goarch); err != nil {
// 		return err
// 	}
// 	if err := writer.WriteField("checksum", artifact.Extra.Checksum); err != nil {
// 		return err
// 	}

// 	// Add the file to the form
// 	part, err := writer.CreateFormFile("file", filepath.Base(artifact.Name))
// 	if err != nil {
// 		fmt.Println(err)
// 		return err
// 	}
// 	_, err = io.Copy(part, r)
// 	if err != nil {
// 		fmt.Println(err)
// 		return err
// 	}

// 	// Close the form
// 	err = writer.Close()
// 	if err != nil {
// 		fmt.Println(err)
// 		return err
// 	}

// 	// Create the HTTP request
// 	req, err := http.NewRequest("POST", FlyPkgsEndpoint+"/"+version+"/assets/"+artifact.Goos+"/"+artifact.Goarch, body)
// 	if err != nil {
// 		fmt.Println(err)
// 		return err
// 	}
// 	req.Header.Set("Content-Type", writer.FormDataContentType())
// 	req.Header.Set("Acceopts", "application/json")

// 	// Send the request
// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		fmt.Println(err)
// 		return err
// 	}
// 	defer resp.Body.Close()

// 	// Read the response body
// 	respBody, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		fmt.Println(err)
// 		return err
// 	}

// 	fmt.Println(string(respBody))

// 	return nil
// }

// version := os.Args[1]
// if version == "" {
// 	version = "latest"
// }

// err := filepath.Walk("dist", func(path string, info os.FileInfo, err error) error {
// 	if err != nil {
// 		return err
// 	}

// 	fmt.Println(path)

// 	if info.IsDir() {
// 		return nil
// 	}

// 	fmt.Println(filepath.Ext(path))

// 	if filepath.Ext(path) != ".zip" && filepath.Ext(path) != ".gz" && filepath.Ext(path) != ".txt" {
// 		fmt.Println("Skipping file", path)
// 		return nil
// 	}

// 	return func() error {
// 		file, err := os.Open(path)
// 		if err != nil {
// 			return err
// 		}
// 		defer file.Close()

// 		return uploadFile(file, version, path)
// 	}()
// })

// if err != nil {
// 	fmt.Println(err)
// }
