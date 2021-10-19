// Package state implements Context.
package state

import "context"

type contextKeyType int

const (
	_ contextKeyType = iota
	appNameKey
)

// WithAppName derives a Context from ctx that carries appName.
func WithAppName(ctx context.Context, appName string) context.Context {
	return context.WithValue(ctx, appNameKey, appName)
}

// AppName returns the app name ctx carries.
//
// AppName panics in case ctx carries no app name.
func AppName(ctx context.Context) string {
	return ctx.Value(appNameKey).(string)
}
