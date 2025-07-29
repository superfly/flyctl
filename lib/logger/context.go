package logger

import "context"

type contextKey struct{}

// NewContext derives a context that carries logger from ctx.
func NewContext(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// FromContext returns the Logger ctx carries. It panics in case ctx carries
// no Logger.
func FromContext(ctx context.Context) *Logger {
	return ctx.Value(contextKey{}).(*Logger)
}

func MaybeFromContext(ctx context.Context) (l *Logger) {
	if v := ctx.Value(contextKey{}); v != nil {
		l = v.(*Logger)
	}

	return
}
