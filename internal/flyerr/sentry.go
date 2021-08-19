package flyerr

import "github.com/getsentry/sentry-go"

type CaptureOption func(scope *sentry.Scope)

func WithContext(key string, val interface{}) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetContext(key, val)
	}
}

func WithContexts(contexts map[string]interface{}) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetContexts(contexts)
	}
}

func WithTag(key, value string) CaptureOption {
	return func(scope *sentry.Scope) {
		scope.SetTag(key, value)
	}
}

func CaptureException(err error, opts ...CaptureOption) {
	sentry.WithScope(func(scope *sentry.Scope) {
		for _, opt := range opts {
			opt(scope)
		}
		sentry.CaptureException(err)
	})
}

func CaptureMessage(msg string, opts ...CaptureOption) {
	sentry.WithScope(func(scope *sentry.Scope) {
		for _, opt := range opts {
			opt(scope)
		}
		sentry.CaptureMessage(msg)
	})
}
