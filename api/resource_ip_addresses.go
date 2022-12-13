package api

import (
	"context"
	"net"
	"time"
)

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
				sharedIpAddress
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	ips := data.App.IPAddresses.Nodes

	// ugly hack
	if data.App.SharedIPAddress != "" {
		ips = append(ips, IPAddress{
			ID:        "",
			Address:   data.App.SharedIPAddress,
			Type:      "shared_v4",
			Region:    "",
			CreatedAt: time.Time{},
		})
	}

	return ips, nil
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

func (c *Client) AllocateIPAddress(ctx context.Context, appName string, addrType string, region string, org *Organization) (*IPAddress, error) {
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
	input := AllocateIPAddressInput{AppID: appName, Type: addrType, Region: region}

	if org != nil {
		input.OrganizationID = org.ID
	}

	req.Var("input", input)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.AllocateIPAddress.IPAddress, nil
}

func (c *Client) AllocateSharedIPAddress(ctx context.Context, appName string) (net.IP, error) {
	query := `
		mutation($input: AllocateIPAddressInput!) {
			allocateIpAddress(input: $input) {
				app {
					sharedIpAddress
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", AllocateIPAddressInput{AppID: appName, Type: "shared_v4"})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return net.ParseIP(data.AllocateIPAddress.App.SharedIPAddress), nil
}

func (c *Client) ReleaseIPAddress(ctx context.Context, appName string, ip string) error {
	query := `
		mutation($input: ReleaseIPAddressInput!) {
			releaseIpAddress(input: $input) {
				clientMutationId
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", ReleaseIPAddressInput{AppID: &appName, IP: &ip})

	_, err := c.RunWithContext(ctx, req)
	if err != nil {
		return err
	}

	return nil
}
