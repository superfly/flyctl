package api

import (
	"context"
	"crypto/ed25519"
	"strings"

	"golang.org/x/crypto/ssh"
)

func (c *Client) GetLoggedCertificates(ctx context.Context, slug string) ([]LoggedCertificate, error) {
	req := c.NewRequest(`
query($slug: String!) {
  organization(slug: $slug) {
    loggedCertificates {
      nodes {
        root
        cert
      }
    }
  }
}
`)
	req.Var("slug", slug)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.Organization.LoggedCertificates.Nodes, nil
}

func (c *Client) EstablishSSHKey(ctx context.Context, org *Organization, override bool) (*SSHCertificate, error) {
	req := c.NewRequest(`
mutation($input: EstablishSSHKeyInput!) {
  establishSshKey(input: $input) {
    certificate
  }
}
`)
	req.Var("input", map[string]interface{}{
		"organizationId": org.ID,
		"override":       override,
	})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.EstablishSSHKey, nil
}

func (c *Client) IssueSSHCertificate(ctx context.Context, org OrganizationImpl, principals []string, apps []App, valid_hours *int, publicKey ed25519.PublicKey) (*IssuedCertificate, error) {
	req := c.NewRequest(`
mutation($input: IssueCertificateInput!) {
  issueCertificate(input: $input) {
    certificate, key
  }
}
`)

	appNames := make([]string, 0, len(apps))
	for _, app := range apps {
		appNames = append(appNames, app.Name)
	}

	var pubStr string
	if len(publicKey) > 0 {
		sshPub, err := ssh.NewPublicKey(publicKey)
		if err != nil {
			return nil, err
		}

		pubStr = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	}

	inputs := map[string]interface{}{
		"organizationId": org.GetID(),
		"principals":     principals,
		"appNames":       appNames,
		"publicKey":      pubStr,
	}

	if valid_hours != nil {
		inputs["validHours"] = *valid_hours
	}

	req.Var("input", inputs)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.IssueCertificate, nil
}
