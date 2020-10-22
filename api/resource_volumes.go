package api

func (c *Client) GetVolumes(appName string) ([]Volume, error) {
	query := `
	query($appName: String!) {
		app(name: $appName) {
			volumes {
				nodes {
					id
					name
					sizeGb
					region
					createdAt
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

	return data.App.Volumes.Nodes, nil
}

func (c *Client) CreateVolume(appName string, volname string, region string, sizeGb int) (*Volume, error) {
	query := `
		mutation($input: CreateVolumeInput!) {
			createVolume(input: $input) {
				app {
					name
				}
				volume {
					id
					name
					region
					sizeGb
					createdAt
				}
			}
		}
	`

	input := CreateVolumeInput{AppID: appName, Name: volname, Region: region, SizeGb: sizeGb}

	req := c.NewRequest(query)

	req.Var("input", input)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreateVolume.Volume, nil
}

func (c *Client) DeleteVolume(volID string) (App *App, err error) {
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

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.DeleteVolume.App, nil
}

func (c *Client) GetVolume(volID string) (Volume *Volume, err error) {
	query := `
	query($id: ID!) {
		Volume(id: $id) {
					id
					name
					sizeGb
					region
					createdAt
		}
	}`

	req := c.NewRequest(query)

	req.Var("id", volID)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.Volume, nil
}
