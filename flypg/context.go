package flypg

import "context"

type cmdContextKey struct{}

func CommandWithContext(ctx context.Context, cmd *Command) context.Context {
	return context.WithValue(ctx, cmdContextKey{}, cmd)
}

func CommandFromContext(ctx context.Context) *Command {
	return ctx.Value(cmdContextKey{}).(*Command)
}
