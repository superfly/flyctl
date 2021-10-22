package api

import "context"

func (c *Client) GetDomains(ctx context.Context, organizationSlug string) ([]*Domain, error) {
	query := `
		query($slug: String!) {
			organization(slug: $slug) {
				domains {
					nodes {
						id
						name
						createdAt
						registrationStatus
						dnsStatus
						autoRenew
						expiresAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("slug", organizationSlug)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return *data.Organization.Domains.Nodes, nil
}

func (c *Client) GetDomain(ctx context.Context, name string) (*Domain, error) {
	query := `
		query($name: String!) {
			domain(name: $name) {
				id
				name
				createdAt
				registrationStatus
				dnsStatus
				autoRenew
				expiresAt
				zoneNameservers
				delegatedNameservers
				organization {
					id
					name
					slug
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("name", name)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.Domain, nil
}

func (c *Client) CreateDomain(organizationID string, name string) (*Domain, error) {
	query := `
		mutation($input: CreateDomainInput!) {
			createDomain(input: $input) {
				domain {
					id
					name
					createdAt
					registrationStatus
					dnsStatus
					autoRenew
					expiresAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]interface{}{
		"organizationId": organizationID,
		"name":           name,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.CreateDomain.Domain, nil
}

func (c *Client) CheckDomain(ctx context.Context, name string) (*CheckDomainResult, error) {
	query := `
		mutation($input: CheckDomainInput!) {
			checkDomain(input: $input) {
				domainName
				tld
				registrationSupported
				registrationAvailable
				registrationPrice
				registrationPeriod
				transferAvailable
				dnsAvailable
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]string{"domainName": name})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.CheckDomain, nil
}

func (c *Client) CreateAndRegisterDomain(organizationID string, name string) (*Domain, error) {
	query := `
		mutation($input: CreateAndRegisterDomainInput!) {
			createAndRegisterDomain(input: $input) {
				domain {
					id
					name
					createdAt
					registrationStatus
					dnsStatus
					autoRenew
					expiresAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]interface{}{
		"organizationId": organizationID,
		"name":           name,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.CreateAndRegisterDomain.Domain, nil
}
