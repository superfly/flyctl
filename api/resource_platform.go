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
					gatewayAvailable
				}
			}
		}
	`

	req := c.NewRequest(query)

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

func (c *Client) PlatformRegionsAll(ctx context.Context) ([]Region, error) {
	query := `
		query {
			platform {
				regions {
					name
					code
					latitude
					longitude
					gatewayAvailable
				}
			}
		}
	`

	req := c.NewRequest(query)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.Platform.Regions, nil
}

func (c *Client) PlatformVMSizes(ctx context.Context) ([]VMSize, error) {
	query := `
		query {
			platform {
				vmSizes {
					name
					cpuCores
					memoryGb
					memoryMb
					priceMonth
					priceSecond
				}
			}
		}
	`

	req := c.NewRequest(query)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.Platform.VMSizes, nil
}
