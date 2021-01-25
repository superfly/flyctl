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

func (client *Client) FindOrganizationBySlug(slug string) (*Organization, error) {
	q := `
		query($slug: String!) {
			organization(slug: $slug) {
				id
				slug
				name
				type
			}
		}
	`

	req := client.NewRequest(q)

	req.Var("slug", slug)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return data.Organization, nil
}

func (client *Client) GetCurrentOrganizations() (Organization, []Organization, error) {
	query := `
	query {
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
	`

	req := client.NewRequest(query)

	data, err := client.Run(req)
	if err != nil {
		return Organization{}, nil, err
	}
	return data.PersonalOrganization, data.Organizations.Nodes, nil
}

func (client *Client) GetOrganizationBySlug(slug string) (*OrganizationDetails, error) {
	query := `query($slug: String!) {
		organizationdetails: organization(slug: $slug) {
		  id
		  slug
		  name
		  type
		  viewerRole
		  members {
				edges {
					cursor
					node {
						id
						name
						email
					}
					joinedAt
					role
				}
		  }
		}
	  }
	`

	req := client.NewRequest(query)
	req.Var("slug", slug)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.OrganizationDetails, nil
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
