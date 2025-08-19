package logs

import (
	"context"
	"time"

	"github.com/azazeal/pause"
	"github.com/pkg/errors"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
)

type pollingStream struct {
	err       error
	apiClient flapsutil.FlapsClient
}

func NewPollingStream(client flapsutil.FlapsClient) LogStream {
	return &pollingStream{apiClient: client}
}

func (s *pollingStream) Stream(ctx context.Context, opts *LogOptions) <-chan fly.LogEntry {
	out := make(chan fly.LogEntry)

	go func() {
		defer close(out)

		s.err = Poll(ctx, out, s.apiClient, opts)
	}()

	return out
}

func (s *pollingStream) Err() error {
	return s.err
}

func Poll(ctx context.Context, out chan<- fly.LogEntry, client flapsutil.FlapsClient, opts *LogOptions) error {
	const (
		minWait = time.Millisecond << 6
		maxWait = minWait << 6
	)

	var (
		errorCount int
		waitFor    = minWait
	)

	for {
		if waitFor > minWait {
			pause.For(ctx, waitFor)
		}

		entries, err := client.GetLogs(ctx, opts.VMID)
		if err != nil {
			switch errorCount++; {
			default:
				waitFor = backoff(waitFor, maxWait)

				continue
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return err
			case fly.IsNotAuthenticatedError(err), fly.IsNotFoundError(err):
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

		for _, entry := range entries {
			out <- entry.LogEntry()
		}

		if opts.NoTail {
			return nil
		}
	}
}

func backoff(current, max time.Duration) (val time.Duration) {
	if val = current << 1; current > max {
		val = max
	}
	return
}
