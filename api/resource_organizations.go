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
