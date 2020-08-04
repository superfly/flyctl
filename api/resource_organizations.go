package api

func (client *Client) GetOrganizations() ([]Organization, error) {
	q := `
		{
			organizations {
				nodes {
					id
					slug
					name
					type
				}
			}
		}
	`

	req := client.NewRequest(q)

	data, err := client.Run(req)
	if err != nil {
		return []Organization{}, err
	}

	return data.Organizations.Nodes, nil
}

func (client *Client) GetOrganization(slug string) ([]Organization, error) {
	q := `
		{
			organizations {
				nodes {
					id
					slug
					name
					type
				}
			}
		}
	`

	req := client.NewRequest(q)

	data, err := client.Run(req)
	if err != nil {
		return []Organization{}, err
	}

	return data.Organizations.Nodes, nil
}

func (client *Client) GetCurrentOrganizations() (Organization, []Organization, error) {
	query := `
		query {
			userOrganizations:currentUser {
				personalOrganization {
				  id
				  slug
				  name
				  type
				  viewerRole
				}
				organizations {
				  nodes {
					id
					slug
					name
					type
					viewerRole
				  }
				}
			  }
		}
	`

	req := client.NewRequest(query)

	data, err := client.Run(req)
	if err != nil {
		return Organization{}, nil, err
	}

	return data.UserOrganizations.PersonalOrganization, data.UserOrganizations.Organizations.Nodes, nil
}

func (c *Client) CreateOrganization(organizationname string) (*Organization, error) {
	query := `
		mutation($input: CreateOrganizationInput!) {
			createOrganization(input: $input) {
			    organization {
					id
					name
					slug
					type
					viewerRole
				  }
			}	
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]string{
		"name": organizationname,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreateOrganization.Organization, nil
}

func (c *Client) DeleteOrganization(id string) (deletedid string, err error) {
	query := `
	mutation($input: DeleteOrganizationInput!) {
		deleteOrganization(input: $input) {
		  clientMutationId
		  deletedOrganizationId
		  }
		}	  
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]string{
		"organizationId": id,
	})

	data, err := c.Run(req)
	if err != nil {
		return "", err
	}

	return data.DeleteOrganization.DeletedOrganizationId, nil
}
