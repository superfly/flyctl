package api

import "context"

func (c *Client) GetAppNameFromVolume(ctx context.Context, volID string) (*string, error) {
	query := `
query($id: ID!) {
	volume: node(id: $id) {
		... on Volume {
			app {
				name
			}
		}
	}
}
	`

	req := c.NewRequest(query)

	req.Var("id", volID)
	ctx = ctxWithAction(ctx, "get_app_name_from_volume")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.Volume.App.Name, nil
}
