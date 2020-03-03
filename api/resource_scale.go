package api

func (c *Client) ScaleApp(appID string, regions []ScaleRegionInput) ([]ScaleRegionChange, error) {
	query := `
		mutation ($input: ScaleAppInput!) {
			scaleApp(input: $input) {
				placement {
					region
					count
				}
				delta {
					region
					fromCount
					toCount
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", ScaleAppInput{AppID: appID, Regions: regions})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.ScaleApp.Delta, nil
}

func (c *Client) UpdateAutoscaleConfig(input UpdateAutoscaleConfigInput) (*AutoscalingConfig, error) {
	query := `
		mutation ($input: UpdateAutoscaleConfigInput!) {
			updateAutoscaleConfig(input: $input) {
				app {
					autoscaling {
						enabled
						minCount
						maxCount
						balanceRegions
						regions {
							code
							minCount
							weight
						}
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", input)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.UpdateAutoscaleConfig.App.Autoscaling, nil
}

func (c *Client) AppAutoscalingConfig(appID string) (*AutoscalingConfig, error) {
	query := `
		query($appId: String!) {
			app(id: $appId) {
				autoscaling {
					enabled
					minCount
					maxCount
					balanceRegions
					regions {
						code
						minCount
						weight
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appId", appID)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Autoscaling, nil
}

func (c *Client) AppVMSize(appID string) (VMSize, error) {
	query := `
		query($appId: String!) {
			app(id: $appId) {
				vmSize {
					name
					cpuCores
					memoryGb
					memoryMb
					priceMonth
					priceSecond
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appId", appID)

	data, err := c.Run(req)
	if err != nil {
		return VMSize{}, err
	}

	return data.App.VMSize, nil
}

func (c *Client) SetAppVMSize(appID string, sizeName string) (VMSize, error) {
	query := `
		mutation ($input: SetVMSizeInput!) {
			setVmSize(input: $input) {
				vmSize {
					name
					cpuCores
					memoryGb
					memoryMb
					priceMonth
					priceSecond
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", SetVMSizeInput{AppID: appID, SizeName: sizeName})

	data, err := c.Run(req)
	if err != nil {
		return VMSize{}, err
	}

	return *data.SetVMSize.VMSize, nil
}
