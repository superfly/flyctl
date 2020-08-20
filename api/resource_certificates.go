package api

func (c *Client) GetAppCertificates(appName string) ([]AppCertificateCompact, error) {
	query := `
		query($appName: String!) {
			appcertscompact:app(name: $appName) {
				certificates {
					nodes {
						createdAt
						hostname
						clientStatus
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.AppCertsCompact.Certificates.Nodes, nil
}

func (c *Client) CheckAppCertificate(appName, hostname string) (*AppCertificate, *HostnameCheck, error) {
	query := `
		mutation($input: CheckCertificateInput!) {
			checkCertificate(input: $input) {
				certificate {
					acmeDnsConfigured
					acmeAlpnConfigured
					configured
					certificateAuthority
					createdAt
					dnsProvider
					dnsValidationInstructions
					dnsValidationHostname
					dnsValidationTarget
					hostname
					id
					source
					clientStatus
					isApex
					issued {
						nodes {
							type
							expiresAt
						}
					}
				}
				check {
					aRecords
				   	aaaaRecords
				   	cnameRecords
				   	soa
			   		dnsProvider
			   		dnsVerificationRecord
				 	resolvedAddresses
			   }
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]string{
		"appId":    appName,
		"hostname": hostname,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, nil, err
	}

	return data.CheckCertificate.Certificate, data.CheckCertificate.Check, nil
}

func (c *Client) AddCertificate(appName, hostname string) (*AppCertificate, *HostnameCheck, error) {
	query := `
		mutation($appId: ID!, $hostname: String!) {
			addCertificate(appId: $appId, hostname: $hostname) {
				certificate {
					acmeDnsConfigured
					acmeAlpnConfigured
					configured
					certificateAuthority
					createdAt
					dnsProvider
					dnsValidationInstructions
					dnsValidationHostname
					dnsValidationTarget
					hostname
					id
					source
					clientStatus
					isApex
					issued {
						nodes {
							type
							expiresAt
						}
					}
				}
				check {
					aRecords
					aaaaRecords
					cnameRecords
					soa
					dnsProvider
					dnsVerificationRecord
				  	resolvedAddresses
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appId", appName)
	req.Var("hostname", hostname)

	data, err := c.Run(req)
	if err != nil {
		return nil, nil, err
	}

	return data.AddCertificate.Certificate, data.AddCertificate.Check, nil
}

func (c *Client) DeleteCertificate(appName, hostname string) (*DeleteCertificatePayload, error) {
	query := `
		mutation($appId: ID!, $hostname: String!) {
			deleteCertificate(appId: $appId, hostname: $hostname) {
				app {
					name
				}
				certificate {
					hostname
					id
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appId", appName)
	req.Var("hostname", hostname)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.DeleteCertificate, nil
}
