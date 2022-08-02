package api

import "context"

func (c *Client) ProvisionService(ctx context.Context, serviceType string, orgID string, region string) (*AddOn, error) {
	query := `
		mutation ($input: ProvisionAddOnInput!) {
			provisionAddOn(input: $input) {
				service {
					publicUrl
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", ProvisionAddOnInput{OrganizationId: orgID, Region: region, Type: serviceType})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.ProvisionAddOn.Service, nil
}

func (c *Client) GetAddOn(ctx context.Context, id string) (*AddOn, error) {
	query := `
		query($id: ID!) {
			addOn(id: $id) {
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

	return data.AddOn, nil
}

func (c *Client) GetAddOns(ctx context.Context, serviceType string) ([]AddOn, error) {
	query := `
		query($type: AddOnType) {
			addOns(type: $type) {
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

	return data.AddOns.Nodes, nil
}

func (c *Client) DeleteAddOn(ctx context.Context, id string) (servicePayload *DeleteAddOnPayload, err error) {
	query := `
		mutation($input: DeleteAddOnInput!) {
			deleteAddOn(input: $input) {
				addOn {
					id
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]string{
		"addOnId": id,
	})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.DeleteAddOn, nil
}
