package flaps

import "context"

type contextKey struct{}
type machineIDCtxKey struct{}
type actionCtxKey struct{}

// NewContext derives a Context that carries c from ctx.
func NewContext(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// FromContext returns the Client ctx carries. It panics in case ctx carries
// no Client.
func FromContext(ctx context.Context) *Client {
	return ctx.Value(contextKey{}).(*Client)
}

func contextWithMachineID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, machineIDCtxKey{}, id)
}

func machineIDFromContext(ctx context.Context) string {
	value := ctx.Value(machineIDCtxKey{})
	if value == nil {
		return ""
	}
	return value.(string)
}

func contextWithAction(ctx context.Context, action flapsAction) context.Context {
	return context.WithValue(ctx, actionCtxKey{}, action)
}

func actionFromContext(ctx context.Context) flapsAction {
	value := ctx.Value(actionCtxKey{})
	if value == nil {
		return none
	}
	return value.(flapsAction)
}
