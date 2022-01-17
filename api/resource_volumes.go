package api

import "context"

func (c *Client) GetVolumes(ctx context.Context, appName string) ([]Volume, error) {
	query := `
	query($appName: String!) {
		app(name: $appName) {
			volumes {
				nodes {
					id
					name
					sizeGb
					region
					encrypted
					createdAt
					host{
						id
					}
					attachedAllocation {
						idShort
						taskName
					}
				}
			}
		}
	}
`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Volumes.Nodes, nil
}

func (c *Client) CreateVolume(ctx context.Context, input CreateVolumeInput) (*Volume, error) {
	query := `
		mutation($input: CreateVolumeInput!) {
			createVolume(input: $input) {
				app {
					name
				}
				volume {
					id
					name
					app{
						name
					}
					region
					sizeGb
					encrypted
					createdAt
					host {
						id
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", input)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.CreateVolume.Volume, nil
}

func (c *Client) DeleteVolume(ctx context.Context, volID string) (App *App, err error) {
	query := `
		mutation($input: DeleteVolumeInput!) {
			deleteVolume(input: $input) {
				app {
					name
				}
			}
		}
	`

	input := DeleteVolumeInput{VolumeID: volID}

	req := c.NewRequest(query)

	req.Var("input", input)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.DeleteVolume.App, nil
}

func (c *Client) GetVolume(ctx context.Context, volID string) (Volume *Volume, err error) {
	query := `
	query($id: ID!) {
		volume: node(id: $id) {
			... on Volume {
				id
				app {
					name
				}
				name
				sizeGb
				region
				encrypted
				createdAt
				host {
					id
				}
			}
		}
	}`

	req := c.NewRequest(query)

	req.Var("id", volID)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.Volume, nil
}

func (c *Client) GetVolumeSnapshots(ctx context.Context, volID string) ([]Snapshot, error) {
	query := `
	query($id: ID!) {
		volume: node(id: $id) {
			... on Volume {
				name
				encrypted
				snapshots {
					nodes {
						id
						size
						digest
						createdAt
					}
				}
			}
		}
	}`

	req := c.NewRequest(query)

	req.Var("id", volID)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.Volume.Snapshots.Nodes, nil
}
