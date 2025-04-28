package logs

import (
	"context"
	"fmt"
	"github.com/apex/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"time"
)

const s3Region = "us-east-1"
const s3Bucket = "fly-app-logs"
const s3RootPrefix = "logs/org_id=%s/"

type s3Stream struct {
	err    error
	client *s3.Client
	opts   *LogOptions
}

func NewS3Stream(ctx context.Context, opts *LogOptions) (LogStream, error) {
	flyClient := flyutil.ClientFromContext(ctx)
	basic, err := flyClient.GetAppBasic(ctx, opts.AppName)
	if err != nil {
		return nil, err
	}
	opts.Org = *basic.Organization
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		OrgSlug: opts.Org.Slug,
	})
	if err != nil {
		return nil, err
	}
	token, err := flapsClient.GetS3LogsToken(ctx, opts.Org.Slug)
	if err != nil {
		return nil, err
	}
	s3Client := s3.NewFromConfig(aws.Config{
		Region: s3Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			token.AccessKeyID,
			token.SecretAccessKey,
			token.SessionToken,
		)},
	)
	return &s3Stream{client: s3Client, opts: opts}, nil
}

func (s *s3Stream) Stream(ctx context.Context, opts *LogOptions) <-chan LogEntry {
	s.opts = opts
	if s.opts.End.IsZero() {
		s.opts.End = time.Now()
	}
	out := make(chan LogEntry)
	go func() {
		defer close(out)
		s.err = s.fromS3(ctx, out)
	}()
	return out
}

func (s *s3Stream) Err() error {
	return s.err
}

func (s *s3Stream) fromS3(ctx context.Context, out chan<- LogEntry) error {
	objects, err := s.listObjects(ctx, s3Bucket, fmt.Sprintf(s3RootPrefix, s.opts.Org.InternalNumericID))
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		return nil
	}
	return s.streamObjects(ctx, objects, out)
}
