package logs

import (
	"context"
	"fmt"
	"io"
	"time"
)

type LogOptions struct {
	W io.Writer

	MaxBackoff time.Duration
	AppName    string
	VMID       string
	RegionCode string
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
