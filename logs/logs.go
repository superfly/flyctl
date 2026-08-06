package logs

import (
	"context"
	"fmt"
	"io"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/wireguard"
)

type LogOptions struct {
	W io.Writer

	MaxBackoff time.Duration
	AppName    string
	VMID       string
	RegionCode string
	NoTail     bool
}

type WebClient interface {
	GetAppBasic(ctx context.Context, appName string) (*fly.AppBasic, error)
	GetAppLogs(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error)
	wireguard.WebClient
}

func (opts *LogOptions) toNatsSubject() (subject string) {
	subject = fmt.Sprintf("logs.%s", opts.AppName)

	add := func(what string) {
		if what == "" {
			what = "*"
		}

		subject = fmt.Sprintf("%s.%s", subject, what)
	}

	add(opts.RegionCode)
	add(opts.VMID)

	return
}

type LogStream interface {
	Err() error
	Stream(ctx context.Context, opts *LogOptions) <-chan LogEntry
}
