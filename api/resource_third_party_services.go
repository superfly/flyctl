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

func (c *Client) GetThirdPartyService(ctx context.Context, id string) (*ThirdPartyService, error) {
	query := `
		query($id: ID!) {
			thirdPartyService(id: $id) {
				id
				name
				primaryRegion
				publicUrl
				organization {
					id
					slug
			}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("id", id)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.ThirdPartyService, nil
}

func (c *Client) GetThirdPartyServices(ctx context.Context, serviceType string) ([]ThirdPartyService, error) {
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

func (c *Client) DeleteThirdPartyService(ctx context.Context, id string) (servicePayload *DeleteThirdPartyServicePayload, err error) {
	query := `
		mutation($input: DeleteThirdPartyServiceInput!) {
			deleteThirdPartyService(input: $input) {
				thirdPartyService {
					id
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]string{
		"thirdPartyServiceId": id,
	})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.DeleteThirdPartyService, nil
}
