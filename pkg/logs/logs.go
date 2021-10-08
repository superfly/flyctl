package logs

import (
	"context"
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

type LogStream interface {
	Err() error
	Stream(ctx context.Context, opts *LogOptions) <-chan LogEntry
}
