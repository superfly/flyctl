package api

import "context"

func (c *Client) ProvisionService(ctx context.Context, serviceType string, orgID string, region string) (*ThirdPartyService, error) {
	query := `
		mutation ($input: ProvisionThirdPartyServiceInput!) {
			provisionThirdPartyService(input: $input) {
				service {
					publicUrl
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", ProvisionThirdPartyServiceInput{OrganizationId: orgID, Region: region, Type: serviceType})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.ProvisionThirdPartyService.Service, nil
}

func (c *Client) GetServices(ctx context.Context, serviceType string) ([]ThirdPartyService, error) {
	query := `
		query($type: ThirdPartyServiceType) {
			thirdPartyServices(type: $type) {
				nodes {
					id
					name
					primaryRegion
					organization {
						id
						slug
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("type", serviceType)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.ThirdPartyServices.Nodes, nil
}
