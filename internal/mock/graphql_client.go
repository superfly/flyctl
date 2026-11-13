package mock

import (
	"context"

	"github.com/Khan/genqlient/graphql"
)

// GraphQLClient lets tests inspect generated requests and supply API responses.
type GraphQLClient func(context.Context, *graphql.Request, *graphql.Response) error

func (f GraphQLClient) MakeRequest(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
	return f(ctx, req, resp)
}
