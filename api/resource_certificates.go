package api

func (c *Client) GetAppCertificates(appName string) ([]AppCertificate, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
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

	return data.App.Certificates.Nodes, nil
}

func (c *Client) GetAppCertificate(appName string, hostname string) (*AppCertificate, error) {
	query := `
		query($appName: String!, $hostname: String!) {
			app(name: $appName) {
				certificate(hostname: $hostname) {
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
					issued {
						nodes {
							type
							expiresAt
						}
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("hostname", hostname)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App.Certificate, nil
}

func (c *Client) CheckAppCertificate(appName string, hostname string) (*AppCertificate, error) {
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
					issued {
						nodes {
							type
							expiresAt
						}
					}
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
		return nil, err
	}

	return data.CheckCertificate.Certificate, nil
}

func (c *Client) AddCertificate(appName string, hostname string) (*AppCertificate, error) {
	query := `
		mutation($appId: ID!, $hostname: String!) {
			addCertificate(appId: $appId, hostname: $hostname) {
				certificate {
					acmeDnsConfigured
					acmeAlpnConfigured
					configured
					certificateAuthority
					certificateRequestedAt
					dnsProvider
					dnsValidationTarget
					hostname
					id
					source
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

	return &data.AddCertificate.Certificate, nil
}

func (c *Client) DeleteCertificate(appName string, hostname string) (*DeleteCertificatePayload, error) {
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
