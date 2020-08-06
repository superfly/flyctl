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
