package api

import "context"

func (c *Client) GetIPAddresses(ctx context.Context, appName string) ([]IPAddress, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				ipAddresses {
					nodes {
						id
						address
						type
						region
						createdAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.IPAddresses.Nodes, nil
}

func (c *Client) FindIPAddress(ctx context.Context, appName string, address string) (*IPAddress, error) {
	query := `
		query($appName: String!, $address: String!) {
			app(name: $appName) {
				ipAddress(address: $address) {
					id
					address
					type
					region
					createdAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("address", address)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.IPAddress, nil
}

func (c *Client) AllocateIPAddress(ctx context.Context, appName string, addrType string, region string) (*IPAddress, error) {
	query := `
		mutation($input: AllocateIPAddressInput!) {
			allocateIpAddress(input: $input) {
				ipAddress {
					id
					address
					type
					region
					createdAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", AllocateIPAddressInput{AppID: appName, Type: addrType, Region: region})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.AllocateIPAddress.IPAddress, nil
}

func (c *Client) ReleaseIPAddress(ctx context.Context, id string) error {
	query := `
		mutation($input: ReleaseIPAddressInput!) {
			releaseIpAddress(input: $input) {
				clientMutationId
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", ReleaseIPAddressInput{IPAddressID: id})

	_, err := c.RunWithContext(ctx, req)
	if err != nil {
		return err
	}

	return nil
}
