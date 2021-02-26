package api

import "fmt"

func (c *Client) GetWireGuardPeers(slug string) ([]*WireGuardPeer, error) {
	req := c.NewRequest(`
query($slug: String!) { 
  organization(slug: $slug) { 
    wireGuardPeers { 
      nodes { 
        id
        name
        pubkey
        region
        peerip
      } 
    }
  } 
}
`)
	req.Var("slug", slug)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return *data.Organization.WireGuardPeers.Nodes, nil
}

func (c *Client) CreateWireGuardPeer(org *Organization, region, name, pubkey string) (*CreatedWireGuardPeer, error) {
	req := c.NewRequest(`
mutation($input: AddWireGuardPeerInput!) { 
  addWireGuardPeer(input: $input) { 
    peerip
    endpointip
    pubkey
  } 
}
`)
	req.Var("input", map[string]interface{}{
		"organizationId": org.ID,
		"region":         region,
		"name":           name,
		"pubkey":         pubkey,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.AddWireGuardPeer, nil
}

func (c *Client) RemoveWireGuardPeer(org *Organization, name string) error {
	req := c.NewRequest(`
mutation($input: RemoveWireGuardPeerInput!) { 
  removeWireGuardPeer(input: $input) { 
    organization { 
      id 
    } 
  }
}
`)
	req.Var("input", map[string]interface{}{
		"organizationId": org.ID,
		"name":           name,
	})

	_, err := c.Run(req)

	return err
}

func (c *Client) CreateDelegatedWireGuardToken(org *Organization, name string) (*DelegatedWireGuardToken, error) {
	req := c.NewRequest(`
mutation($input: CreateDelegatedWireGuardTokenInput!) { 
  createDelegatedWireGuardToken(input: $input) { 
    token
  } 
}
`)
	req.Var("input", map[string]interface{}{
		"organizationId": org.ID,
		"name":           name,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreateDelegatedWireGuardToken, nil
}

func (c *Client) DeleteDelegatedWireGuardToken(org *Organization, name, token *string) error {
	query := `
mutation($input: DeleteDelegatedWireGuardTokenInput!) { 
  deleteDelegatedWireGuardToken(input: $input) { 
    token
  }
}
`

	input := map[string]interface{}{
		"organizationId": org.ID,
	}

	if name != nil {
		input["name"] = *name
	} else {
		input["token"] = *token
	}

	fmt.Printf("%+v\n", input)

	req := c.NewRequest(query)
	req.Var("input", input)

	_, err := c.Run(req)

	return err
}

func (c *Client) GetDelegatedWireGuardTokens(slug string) ([]*DelegatedWireGuardTokenHandle, error) {
	req := c.NewRequest(`
query($slug: String!) { 
  organization(slug: $slug) { 
    delegatedWireGuardTokens { 
      nodes { 
        name
      } 
    }
  } 
}
`)
	req.Var("slug", slug)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return *data.Organization.DelegatedWireGuardTokens.Nodes, nil
}
