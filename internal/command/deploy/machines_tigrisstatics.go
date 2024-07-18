package deploy

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/samber/lo"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	extensions "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

// TODO(allison): Delete the statics bucket when the app is deleted.

const staticsKeepVersions = 3

type tigrisStaticsData struct {
	s3              *s3.Client
	bucket          string
	root            string
	originalStatics []appconfig.Static
}

func (md *machineDeployment) staticsUseTigris(ctx context.Context) bool {

	for _, static := range md.appConfig.Statics {
		if staticIsCandidateForTigrisPush(static) {
			return true
		}
	}

	return false
}

func (md *machineDeployment) staticsEnsureBucketCreated(ctx context.Context) error {

	client := flyutil.ClientFromContext(ctx).GenqClient()

	response, err := gql.ListAddOns(ctx, client, "tigris")
	if err != nil {
		return err
	}

	for _, extension := range response.AddOns.Nodes {
		if extension.Name == md.tigrisStatics.bucket {
			return nil
		}
	}

	org, err := client.GetOrganizationBySlug(ctx, md.app.Organization.Slug)
	if err != nil {
		return err
	}

	params := extensions.ExtensionParams{
		Organization:         org,
		Provider:             "tigris",
		Options:              gql.AddOnOptions{},
		ErrorCaptureCallback: nil,
		OverrideRegion:       md.appConfig.PrimaryRegion,
		OverrideName:         &md.tigrisStatics.bucket,
	}
	params.Options["website"] = map[string]interface{}{
		"domain_name": "",
	}
	params.Options["accelerate"] = false
	params.Options["public"] = false

	_, err = extensions.ProvisionExtension(ctx, params)
	return err
}

// Create the tigris bucket if not created.
func (md *machineDeployment) staticsInitialize(ctx context.Context) error {

	md.tigrisStatics.bucket = md.appConfig.AppName + "-statics"

	if err := md.staticsEnsureBucketCreated(ctx); err != nil {
		return err
	}

	// NOTE: This statics definition in the release sent to our API
	//       should be correct and unmodified. *But*, because we're
	//       modifying the app config in-place to ensure we don't have
	//       double definitions for the static (both tigris & from local),
	//       we'll pull an incorrect config if we grab it from machines.
	//
	// TODO(allison): We can probably solve this by sending the full statics config
	//                to each machine as metadata and resynthesizing it during config save.
	md.tigrisStatics.originalStatics = md.appConfig.Statics
	md.appConfig.Statics = lo.Filter(md.appConfig.Statics, func(static appconfig.Static, _ int) bool {
		return !staticIsCandidateForTigrisPush(static)
	})

	// TODO: Initialize the s3 client
	// md.tigrisStatics =

	md.tigrisStatics.root = fmt.Sprintf("fly-statics/%s/%d", md.appConfig.AppName, md.releaseVersion)
	return nil
}

func staticIsCandidateForTigrisPush(static appconfig.Static) bool {
	if static.TigrisBucket != "" {
		// If this is already mapped to a tigris bucket, that means the user is directly
		// controlling the bucket, and therefore we should not touch it or push anything to it.
		return false
	}
	if len(static.GuestPath) == 0 {
		// TODO(allison): Log a warning that the statics path is empty
		return false
	}
	// TODO(allison): Extract statics from the docker image?
	if static.GuestPath[0] == '/' {
		// This is an absolute path. We should not modify this, as this path
		// is going to be relative to the root of the docker image.
		return false
	}
	// Now we know that we have a relative path, and that we're not already using a tigris bucket.
	// We can push this to the bucket.
	return true
}

// Upload a directory to the tigris bucket with the given prefix `dest`.
func (client *tigrisStaticsData) uploadDirectory(ctx context.Context, dest, localPath string) error {
	// Recursively upload the directory to the bucket.
	var files []string
	localDir := os.DirFS(localPath)
	err := fs.WalkDir(localDir, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, name)
		return nil
	})
	if err != nil {
		return err
	}

	// Create a work queue, then start a number of workers to upload the files.
	workQueue := make(chan string, len(files))
	for _, file := range files {
		workQueue <- file
	}
	close(workQueue)

	workerErr := make(chan error, 1)
	workerCtx, cancelWorker := context.WithCancel(ctx)
	wg := sync.WaitGroup{}

	worker := func() {
		defer wg.Done()
		for file := range workQueue {

			reader, err := os.Open(filepath.Join(localPath, file))
			if err != nil {
				workerErr <- err
				cancelWorker()
				return
			}

			if runtime.GOOS == "windows" {
				file = strings.ReplaceAll(file, "\\", "/")
			}

			terminal.Debugf("Uploading to %s\n", path.Join(dest, file))

			// Upload the file to the bucket.
			_, err = client.s3.PutObject(workerCtx, &s3.PutObjectInput{
				Bucket: &client.bucket,
				Key:    fly.Pointer(path.Join(dest, file)),
				Body:   reader,
			})
			if err != nil {
				workerErr <- err
				cancelWorker()
				return
			}

			err = reader.Close()
			if err != nil {
				terminal.Debugf("failed to close file %s: %v", file, err)
			}
		}
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go worker()
	}

	wg.Wait()

	// Check if any of the workers failed.
	select {
	case err := <-workerErr:
		return err
	default:
		return nil
	}
}

// Delete all files with the given prefix `dir` from the bucket.
func (client *tigrisStaticsData) deleteDirectory(ctx context.Context, dir string) error {

	if runtime.GOOS == "windows" {
		dir = strings.ReplaceAll(dir, "\\", "/")
	}

	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	// List all files in the bucket with the given prefix.
	listOutput, err := client.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &client.bucket,
		Prefix: fly.Pointer(dir),
	})
	if err != nil {
		return err
	}

	objectIdentifiers := lo.Map(listOutput.Contents, func(obj types.Object, _ int) types.ObjectIdentifier {
		return types.ObjectIdentifier{
			Key: obj.Key,
		}
	})

	// Delete files in batches of 1000
	split := lo.Chunk(objectIdentifiers, 1000)
	for _, batch := range split {

		_, err = client.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: &client.bucket,
			Delete: &types.Delete{
				Objects: batch,
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (client *tigrisStaticsData) deleteOldStatics(ctx context.Context, appName string) error {

	// TODO(allison): Note the current deployment version, and remove all versions except for current
	//                if there are any versions newer than the current deployment.
	//                Without this, deleting an app then recreating it would prevent uploading
	//                statics to the new app.

	// List directories in the app's directory.
	// Delete all versions except for the three latest versions.

	// TODO(allison): Support pagination if the bucket contains >1k objects.
	//                Right now, this is egregiously incorrect and brittle.
	// List `fly-statics/<app_name>/` to get a list of all versions.
	listOutput, err := client.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    &client.bucket,
		Prefix:    fly.Pointer(fmt.Sprintf("fly-statics/%s/", appName)),
		Delimiter: fly.Pointer("/"),
	})

	if err != nil {
		return err
	}

	// Extract the version numbers from the common prefixes.
	// These should be strings of the format `fly-statics/<app_name>/<version>/`.
	versions := lo.FilterMap(listOutput.CommonPrefixes, func(prefix types.CommonPrefix, _ int) (int, bool) {
		// The number is the third part of the prefix.
		parts := strings.Split(*prefix.Prefix, "/")
		if len(parts) < 3 {
			return 0, false
		}
		num, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, false
		}
		return num, true
	})

	// Sort the numbers in ascending order.
	slices.Sort(versions)

	versions = lo.Uniq(versions)

	// Delete versions that are older than we wish to keep.
	if len(versions) > staticsKeepVersions {
		versions = versions[:len(versions)-staticsKeepVersions]
		for _, version := range versions {
			terminal.Debugf("Deleting old static dir: %s\n", fmt.Sprintf("fly-statics/%s/%d/", appName, version))
			err = client.deleteDirectory(ctx, fmt.Sprintf("fly-statics/%s/%d/", appName, version))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Push statics to the tigris bucket.
func (md *machineDeployment) staticsPush(ctx context.Context) (err error) {

	defer func() {
		panicErr := recover()
		if err != nil || panicErr != nil {
			md.staticsCleanupAfterFailure()
		}
		if panicErr != nil {
			panic(panicErr)
		}
	}()

	staticNum := 0
	for _, static := range md.tigrisStatics.originalStatics {
		if !staticIsCandidateForTigrisPush(static) {
			continue
		}
		dest := fmt.Sprintf("%s/%d", md.tigrisStatics.root, staticNum)
		staticNum += 1

		err = md.tigrisStatics.uploadDirectory(ctx, dest, path.Clean(static.GuestPath))
		if err != nil {
			return err
		}

		// TODO(allison): Remove this hack. We should be creating virtual service definitions instead.
		//                This is a temporary workaround to get something demoable.

		md.appConfig.Statics = append(md.appConfig.Statics, appconfig.Static{
			GuestPath:     dest,
			UrlPrefix:     static.UrlPrefix,
			TigrisBucket:  md.tigrisStatics.bucket,
			IndexDocument: static.IndexDocument,
		})
	}

	return nil
}

// Delete old statics from the tigris bucket.
func (md *machineDeployment) staticsFinalize(ctx context.Context) error {

	io := iostreams.FromContext(ctx)

	// Delete old statics from the bucket.
	err := md.tigrisStatics.deleteOldStatics(ctx, md.appConfig.AppName)
	if err != nil {
		fmt.Fprintf(io.ErrOut, "Failed to delete old statics: %v\n", err)
	}

	// TODO(allison): do we need to do anything else here? i.e. push new service config?
	//                this is dependent on the proxy work to support statics, which I don't
	//                *believe* is done yet.
	//                I presume configuring this would happen after machine deployment,
	//                since you should hypothetically be able to run a static site
	//                off of tigris and zero machines. we'll see :)
	return nil
}

// We failed, let's delete the incomplete push.
func (md *machineDeployment) staticsCleanupAfterFailure() {

	terminal.Debugf("Cleaning up failed statics push\n")

	deleteCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := md.tigrisStatics.deleteDirectory(deleteCtx, md.tigrisStatics.root)
	if err != nil {
		terminal.Debugf("Failed to delete statics: %v\n", err)
	}
}
