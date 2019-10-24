package api

func (c *Client) GetIPAddresses(appName string) ([]IPAddress, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				ipAddresses {
					nodes {
						id
						address
						type
						createdAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.IPAddresses.Nodes, nil
}

func (c *Client) FindIPAddress(appName string, address string) (*IPAddress, error) {
	query := `
		query($appName: String!, $address: String!) {
			app(name: $appName) {
				ipAddress(address: $address) {
					id
					address
					type
					createdAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("address", address)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.IPAddress, nil
}

func (c *Client) AllocateIPAddress(appName string, addrType string) (*IPAddress, error) {
	query := `
		mutation($input: AllocateIPAddressInput!) {
			allocateIpAddress(input: $input) {
				ipAddress {
					id
					address
					type
					createdAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", AllocateIPAddressInput{AppID: appName, Type: addrType})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.AllocateIPAddress.IPAddress, nil
}

func (c *Client) ReleaseIPAddress(id string) error {
	query := `
		mutation($input: ReleaseIPAddressInput!) {
			releaseIpAddress(input: $input) {
				clientMutationId
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", ReleaseIPAddressInput{IPAddressID: id})

	_, err := c.Run(req)
	if err != nil {
		return err
	}

	return nil
}
