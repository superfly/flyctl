package api

func (c *Client) GetDNSRecords(domainName string) ([]*DNSRecord, error) {
	query := `
		query($domainName: String!) {
			domain(name: $domainName) {
				dnsRecords {
					nodes {
						id
						fqdn
						name
						type
						ttl
						rdata
						isApex
						isWildcard
						isSystem
						createdAt
						updatedAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("domainName", domainName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	if data.Domain == nil {
		return nil, ErrNotFound
	}

	return *data.Domain.DnsRecords.Nodes, nil
}

func (c *Client) ExportDNSRecords(domainId string) (string, error) {
	query := `
		mutation($input: ExportDnsZoneInput!) {
			exportDnsZone(input: $input) {
				contents
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]interface{}{
		"domainId": domainId,
	})

	data, err := c.Run(req)
	if err != nil {
		return "", err
	}

	return data.ExportDnsZone.Contents, nil
}

func (c *Client) ImportDNSRecords(domainId string, zonefile string) ([]ImportDnsWarning, []ImportDnsChange, error) {
	query := `
		mutation($input: ImportDnsZoneInput!) {
			importDnsZone(input: $input) {
				changes {
					action
					newText
					oldText
				}
				warnings {
					action
					message
					attributes {
						name
						rdata
						ttl
						type
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]interface{}{
		"domainId": domainId,
		"zonefile": zonefile,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, nil, err
	}

	return data.ImportDnsZone.Warnings, data.ImportDnsZone.Changes, nil
}
