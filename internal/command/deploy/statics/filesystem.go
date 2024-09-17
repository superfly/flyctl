package statics

import (
	"context"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/samber/lo"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/terminal"
)

// Upload a directory to the tigris bucket with the given prefix `dest`.
func (deployer *DeployerState) uploadDirectory(ctx context.Context, dest, localPath string) error {

	// Clean the destination path.
	// This is for the case where someone launches an app, it fails, then they
	// just delete the app and re-launch it.
	if err := deployer.deleteDirectory(ctx, dest); err != nil {
		return err
	}

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

	waitForWorkers := spawnWorkers(ctx, 5, func(ctx context.Context) error {
		for file := range workQueue {

			reader, err := os.Open(filepath.Join(localPath, file))
			if err != nil {
				return err
			}

			mimeType := "application/octet-stream"
			if detectedMime := mime.TypeByExtension(filepath.Ext(file)); detectedMime != "" {
				mimeType = detectedMime
			} else {
				first512 := make([]byte, 512)
				_, err = reader.Read(first512)
				if err != nil {
					return fmt.Errorf("failed to read static file %s: %w", file, err)
				} else {
					_, err = reader.Seek(0, 0)
					if err != nil {
						return fmt.Errorf("failed to seek static file %s: %w", file, err)
					}
					mimeType = http.DetectContentType(first512)
				}
			}

			if runtime.GOOS == "windows" {
				file = strings.ReplaceAll(file, "\\", "/")
			}

			terminal.Debugf("Uploading to %s\n", path.Join(dest, file))

			// Upload the file to the bucket.
			_, err = deployer.s3.PutObject(ctx, &s3.PutObjectInput{
				Bucket:      &deployer.bucket,
				Key:         fly.Pointer(path.Join(dest, file)),
				Body:        reader,
				ContentType: &mimeType,
			})
			if err != nil {
				return err
			}

			err = reader.Close()
			if err != nil {
				terminal.Debugf("failed to close file %s: %v", file, err)
			}
		}
		return nil
	})

	return waitForWorkers()
}

// Delete all files with the given prefix `dir` from the bucket.
func (deployer *DeployerState) deleteDirectory(ctx context.Context, dir string) error {

	if runtime.GOOS == "windows" {
		dir = strings.ReplaceAll(dir, "\\", "/")
	}

	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	paginator := s3.NewListObjectsV2Paginator(deployer.s3, &s3.ListObjectsV2Input{
		Bucket:    &deployer.bucket,
		Prefix:    fly.Pointer(dir),
		Delimiter: fly.Pointer("/"),
	})

	for paginator.HasMorePages() {

		listOutput, err := paginator.NextPage(ctx)
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

			_, err = deployer.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: &deployer.bucket,
				Delete: &types.Delete{
					Objects: batch,
				},
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}
