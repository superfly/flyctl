package monitor

import (
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

func WatchLogs(ctx *cmdctx.CmdContext, w io.Writer, opts LogOptions) error {
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
		entries, token, err := ctx.Client.API().GetAppLogs(opts.AppName, nextToken, opts.RegionCode, opts.VMID)

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
