package api

import (
	"context"
	"fmt"
	"os"
)

func (c *Client) GetWireGuardPeer(ctx context.Context, slug, name string) (*WireGuardPeer, error) {
	req := c.NewRequest(`
query($slug: String!, $name: String!) {
  organization(slug: $slug) {
    wireGuardPeer(name: $name) {
      id
      name
      pubkey
      region
      peerip
    }
  }
}
`)
	req.Var("slug", slug)
	req.Var("name", name)
	ctx = ctxWithAction(ctx, "get_wg_peer")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	// this graphql code is satanic
	return data.Organization.WireGuardPeer, nil
}

func (c *Client) GetWireGuardPeers(ctx context.Context, slug string) ([]*WireGuardPeer, error) {
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
	ctx = ctxWithAction(ctx, "get_wg_peers")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return *data.Organization.WireGuardPeers.Nodes, nil
}

func (c *Client) CreateWireGuardPeer(ctx context.Context, org *Organization, region, name, pubkey string) (*CreatedWireGuardPeer, error) {
	req := c.NewRequest(`
mutation($input: AddWireGuardPeerInput!) {
  addWireGuardPeer(input: $input) {
    peerip
    endpointip
    pubkey
  }
}
`)

	var nats bool

	if os.Getenv("WG_NATS") != "" {
		nats = true
		fmt.Printf("Creating wiregard peer via NATS")
	}

	inputs := map[string]interface{}{
		"organizationId": org.ID,
		"name":           name,
		"pubkey":         pubkey,
		"nats":           nats,
	}

	if region != "" {
		inputs["region"] = region
	}

	req.Var("input", inputs)
	ctx = ctxWithAction(ctx, "create_wg_peers")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.AddWireGuardPeer, nil
}

func (c *Client) RemoveWireGuardPeer(ctx context.Context, org *Organization, name string) error {
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
	ctx = ctxWithAction(ctx, "remove_wg_peer")

	_, err := c.RunWithContext(ctx, req)

	return err
}

func (c *Client) CreateDelegatedWireGuardToken(ctx context.Context, org *Organization, name string) (*DelegatedWireGuardToken, error) {
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
	ctx = ctxWithAction(ctx, "create_deletegated_wg_token")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.CreateDelegatedWireGuardToken, nil
}

func (c *Client) DeleteDelegatedWireGuardToken(ctx context.Context, org *Organization, name, token *string) error {
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
	ctx = ctxWithAction(ctx, "delete_deletegated_wg_token")

	_, err := c.RunWithContext(ctx, req)

	return err
}

func (c *Client) GetDelegatedWireGuardTokens(ctx context.Context, slug string) ([]*DelegatedWireGuardTokenHandle, error) {
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
	ctx = ctxWithAction(ctx, "get_deletegated_wg_tokens")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return *data.Organization.DelegatedWireGuardTokens.Nodes, nil
}

func (c *Client) ClosestWireguardGatewayRegion(ctx context.Context) (*Region, error) {
	req := c.NewRequest(`
		query {
			nearestRegion(wireguardGateway: true) {
				code
				name
				gatewayAvailable
			}
		}
`)
	ctx = ctxWithAction(ctx, "closest_wg_gateway_region")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.NearestRegion, nil
}

func (c *Client) ValidateWireGuardPeers(ctx context.Context, peerIPs []string) (invalid []string, err error) {
	req := c.NewRequest(`
mutation($input: ValidateWireGuardPeersInput!) {
  validateWireGuardPeers(input: $input) {
		invalidPeerIps
	}
}
`)

	req.Var("input", map[string]interface{}{
		"peerIps": peerIPs,
	})
	ctx = ctxWithAction(ctx, "validate_wg_peers")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.ValidateWireGuardPeers.InvalidPeerIPs, nil
}
