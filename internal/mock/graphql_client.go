package mock

import (
	"context"

	genq "github.com/Khan/genqlient/graphql"
)

type GraphQLClient struct {
	MakeRequestFunc func(ctx context.Context, req *genq.Request, resp *genq.Response) error
}

func (m *GraphQLClient) MakeRequest(ctx context.Context, req *genq.Request, resp *genq.Response) error {
	return m.MakeRequestFunc(ctx, req, resp)
}
