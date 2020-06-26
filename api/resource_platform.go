package api

func (c *Client) PlatformRegions() ([]Region, error) {
	query := `
		query {
			platform {
				regions {
					name
					code
				}
			}
		}
	`

	req := c.NewRequest(query)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.Platform.Regions, nil
}

func (c *Client) PlatformRegionsAll() ([]Region, error) {
	query := `
		query {
			platform {
				regions {
					name
					code
					latitude
					longitude
				}
			}
		}
	`

	req := c.NewRequest(query)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.Platform.Regions, nil
}

func (c *Client) PlatformVMSizes() ([]VMSize, error) {
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

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.Platform.VMSizes, nil
}
