package deployment

import "context"

type deploymentIdKey struct{}

func WithDeployID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, deploymentIdKey{}, id)
}

func DeployIDFromContext(ctx context.Context) string {
	return ctx.Value(deploymentIdKey{}).(string)
}
