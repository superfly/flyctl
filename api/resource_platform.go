package api

import "context"

func (c *Client) PlatformRegions(ctx context.Context) ([]Region, *Region, error) {
	query := `
		query {
			platform {
				requestRegion
				regions {
					name
					code
					latitude
					longitude
					gatewayAvailable
					requiresPaidPlan
				}
			}
		}
	`

	req := c.NewRequest(query)
	ctx = ctxWithAction(ctx, "platform_regions")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	var requestRegion *Region

	if data.Platform.RequestRegion != "" {
		for _, region := range data.Platform.Regions {
			if region.Code == data.Platform.RequestRegion {
				requestRegion = &region
				break
			}
		}
	}

	return data.Platform.Regions, requestRegion, nil
}
