package monitor

import (
	"context"
	"io"
	"time"

	"github.com/jpillora/backoff"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
)

type LogOptions struct {
	W io.Writer

	MaxBackoff time.Duration
	AppName    string
	VMID       string
	RegionCode string
}

func WatchLogs(cc *cmdctx.CmdContext, w io.Writer, opts LogOptions) error {
	errorCount := 0

	b := &backoff.Backoff{
		Min:    250 * time.Millisecond,
		Max:    5 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	if opts.MaxBackoff != 0 {
		b.Max = opts.MaxBackoff
	}

	nextToken := ""

	logPresenter := presenters.LogPresenter{}

	for {
		entries, token, err := cc.Client.API().GetAppLogs(opts.AppName, nextToken, opts.RegionCode, opts.VMID)

		if err != nil {
			if api.IsNotAuthenticatedError(err) {
				return err
			} else if api.IsNotFoundError(err) {
				return err
			} else {
				errorCount++
				if errorCount > 10 {
					return err
				}
				time.Sleep(b.Duration())
			}
		}
		errorCount = 0

		if len(entries) == 0 {
			time.Sleep(b.Duration())
		} else {
			b.Reset()

			logPresenter.FPrint(w, false, entries)

			if token != "" {
				nextToken = token
			}
		}
	}
}

func NewLogStream(apiClient *api.Client) *LogStream {
	return &LogStream{apiClient: apiClient}
}

type LogStream struct {
	apiClient *api.Client
	err       error
}

func (ls *LogStream) Err() error {
	return ls.err
}

func (ls *LogStream) Stream(ctx context.Context, opts LogOptions) <-chan []api.LogEntry {
	out := make(chan []api.LogEntry)

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
			entries, token, err := ls.apiClient.GetAppLogs(opts.AppName, nextToken, opts.RegionCode, opts.VMID)

			if err != nil {
				errorCount++

				if api.IsNotAuthenticatedError(err) || api.IsNotFoundError(err) || errorCount > 10 {
					ls.err = err
					return
				}
				wait = time.After(b.Duration())
			} else {
				errorCount = 0

				if len(entries) == 0 {
					wait = time.After(b.Duration())
				} else {
					b.Reset()

					out <- entries
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
