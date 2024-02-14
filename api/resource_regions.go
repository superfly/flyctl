package api

import "context"

func (c *Client) GetNearestRegion(ctx context.Context) (*Region, error) {
	req := c.NewRequest(`
		query {
			nearestRegion {
				code
				name
				gatewayAvailable
			}
		}
`)

	ctx = ctxWithAction(ctx, "get_nearest_regions")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.NearestRegion, nil
}
