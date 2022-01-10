package state

import (
	"context"
	"log"
)

type contextKeyType int

const (
	_ contextKeyType = iota
	loggerKey
	daemonKey
)

func WithLogger(ctx context.Context, l *log.Logger) context.Context {
	return set(ctx, loggerKey, l)
}

func Logger(ctx context.Context) *log.Logger {
	return get(ctx, loggerKey).(*log.Logger)
}

func WithDaemon(ctx context.Context, d bool) context.Context {
	return set(ctx, daemonKey, d)
}

func Daemon(ctx context.Context) bool {
	return get(ctx, daemonKey).(bool)
}

func get(ctx context.Context, key contextKeyType) interface{} {
	return ctx.Value(key)
}

func set(ctx context.Context, key contextKeyType, val interface{}) context.Context {
	return context.WithValue(ctx, key, val)
}
