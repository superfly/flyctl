package logs

import (
	"context"
	"time"

	"github.com/azazeal/pause"
	"github.com/pkg/errors"

	"github.com/superfly/flyctl/api"
)

type pollingStream struct {
	err       error
	apiClient *api.Client
}

func NewPollingStream(client *api.Client, opts *LogOptions) (LogStream, error) {
	return &pollingStream{apiClient: client}, nil
}

func (s *pollingStream) Stream(ctx context.Context, opts *LogOptions) <-chan LogEntry {
	out := make(chan LogEntry)

	go func() {
		defer close(out)

		s.err = Poll(ctx, out, s.apiClient, opts)
	}()

	return out
}

func (s *pollingStream) Err() error {
	return s.err
}

func Poll(ctx context.Context, out chan<- LogEntry, client *api.Client, opts *LogOptions) error {
	const (
		minWait = time.Millisecond << 6
		maxWait = minWait << 6
	)

	var (
		errorCount int
		nextToken  string
		waitFor    = minWait
	)

	for {
		if waitFor > minWait {
			pause.For(ctx, waitFor)
		}

		entries, token, err := client.GetAppLogs(ctx, opts.AppName, nextToken, opts.RegionCode, opts.VMID)
		if err != nil {
			switch errorCount++; {
			default:
				waitFor = backoff(waitFor, maxWait)

				continue
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return err
			case api.IsNotAuthenticatedError(err), api.IsNotFoundError(err):
				return err
			case errorCount > 9:
				return err
			}
		}

		errorCount = 0
		if len(entries) == 0 {
			waitFor = backoff(minWait, maxWait)

			continue
		}

		waitFor = 0

		if token != "" {
			nextToken = token
		}

		for _, entry := range entries {
			out <- LogEntry{
				Instance:  entry.Instance,
				Level:     entry.Level,
				Message:   entry.Message,
				Region:    entry.Region,
				Timestamp: entry.Timestamp,
				Meta:      entry.Meta,
			}
		}
	}
}

func backoff(current, max time.Duration) (val time.Duration) {
	if val = current << 1; current > max {
		val = max
	}
	return
}
