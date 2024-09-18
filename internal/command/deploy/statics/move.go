package statics

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/samber/lo"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

// MoveBucket moves the statics bucket from one org to another.
// Or, more precisely, it creates a new bucket in the new org and copies
// all the files from the old bucket to the new bucket - then deletes the old bucket.
func MoveBucket(
	ctx context.Context,
	prevBucket *gql.ListAddOnsAddOnsAddOnConnectionNodesAddOn,
	prevOrg *fly.Organization,
	app *fly.App,
	targetOrg *fly.Organization,
	machines []*fly.Machine,
) error {

	// There should probably be a better way to move a bucket between orgs.

	client := flyutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)

	appConfig, err := appconfig.FromRemoteApp(ctx, app.Name)
	if err != nil {
		return err
	}

	prevBucketMeta := prevBucket.Metadata.(map[string]interface{})
	prevBucketAuth := prevBucketMeta[staticsMetaTokenizedAuth].(string)
	oldBucketS3Client, err := s3ClientWithAuth(ctx, prevBucketAuth, prevOrg)
	if err != nil {
		return err
	}

	prevBucketName := prevBucketMeta[staticsMetaBucketName].(string)

	deployer := Deployer(appConfig, app, targetOrg, app.CurrentRelease.Version)
	err = deployer.Configure(ctx)
	if err != nil {
		return err
	}

	if deployer.bucket == prevBucketName {
		fmt.Fprintf(io.ErrOut, "New statics bucket is the same as the old one!\nPlease delete the storage addon '%s' manually and redeploy the application.\n", prevBucket.Name)
		return nil
	}

	err = transferFiles(ctx, oldBucketS3Client, prevBucketName, deployer.s3, deployer.bucket)
	if err != nil {
		return err
	}

	_, err = gql.DeleteAddOn(ctx, client.GenqClient(), prevBucket.Name)
	if err != nil {
		return err
	}

	for _, machine := range machines {
		for _, st := range machine.Config.Statics {
			if st.TigrisBucket == prevBucketName {
				st.TigrisBucket = deployer.bucket
			}
		}
	}

	fmt.Fprintf(io.Out, "migrated statics from %s to %s\n", prevBucketName, deployer.bucket)

	return nil
}

func transferFiles(ctx context.Context, oldS3Client *s3.Client, oldBucket string, newS3Client *s3.Client, newBucket string) error {

	const workerCount = 5

	// Use a work queue to copy files from the old bucket to the new bucket.
	// There should probably be a way to move buckets between orgs, but this
	// is a good enough solution for now.
	workQueue := make(chan string, workerCount*2)

	waitForWorkers := spawnWorkers(ctx, workerCount, func(ctx context.Context) error {
		for key := range workQueue {
			reader, err := oldS3Client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: fly.Pointer(oldBucket),
				Key:    fly.Pointer(key),
			})
			if err != nil {
				return err
			}

			_, err = newS3Client.PutObject(ctx, &s3.PutObjectInput{
				Bucket:      fly.Pointer(newBucket),
				Key:         fly.Pointer(key),
				Body:        reader.Body,
				ContentType: reader.ContentType,
			})

			_ = reader.Body.Close()

			if err != nil {
				return err
			}
		}
		return nil
	})

	paginator := s3.NewListObjectsV2Paginator(oldS3Client, &s3.ListObjectsV2Input{
		Bucket:    fly.Pointer(oldBucket),
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

		for _, file := range objectIdentifiers {
			workQueue <- *file.Key
		}
	}

	close(workQueue)

	return waitForWorkers()
}
