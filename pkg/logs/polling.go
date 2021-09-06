package logs

import (
	"context"
	"time"

	"github.com/jpillora/backoff"
	"github.com/superfly/flyctl/api"
)

type pollingStream struct {
	err       error
	apiClient *api.Client
}

func NewPollingStream(client *api.Client) (LogStream, error) {
	return &pollingStream{apiClient: client}, nil
}

func (p *pollingStream) Stream(ctx context.Context, opts *LogOptions) <-chan LogEntry {
	out := make(chan LogEntry)

	b := &backoff.Backoff{
		Min:    250 * time.Millisecond,
		Max:    5 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	if opts.MaxBackoff != 0 {
		b.Max = opts.MaxBackoff
	}

	go func() {
		defer close(out)
		errorCount := 0
		nextToken := ""

		var wait <-chan time.Time

		for {
			entries, token, err := p.apiClient.GetAppLogs(opts.AppName, nextToken, opts.RegionCode, opts.VMID)

			if err != nil {
				errorCount++

				if api.IsNotAuthenticatedError(err) || api.IsNotFoundError(err) || errorCount > 10 {
					p.err = err
					return
				}
				wait = time.After(b.Duration())
			} else {
				errorCount = 0

				if len(entries) == 0 {
					wait = time.After(b.Duration())
				} else {
					b.Reset()

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
					wait = time.After(0)

					if token != "" {
						nextToken = token
					}
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-wait:
			}
		}
	}()

	return out
}
