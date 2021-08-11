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

func (c *Client) AppAutoscalingConfig(appName string) (*AutoscalingConfig, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
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

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Autoscaling, nil
}

func (c *Client) AppVMResources(appName string) (VMSize, []TaskGroupCount, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				vmSize {
					name
					cpuCores
					memoryGb
					memoryMb
					priceMonth
					priceSecond
				}
				taskGroupCounts {
					name
					count
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return VMSize{}, []TaskGroupCount{}, err
	}

	return data.App.VMSize, data.App.TaskGroupCounts, nil
}

func (c *Client) SetAppVMSize(appID string, sizeName string, memoryMb int64) (VMSize, error) {
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

	req.Var("input", SetVMSizeInput{AppID: appID, SizeName: sizeName, MemoryMb: memoryMb})

	data, err := c.Run(req)
	if err != nil {
		return VMSize{}, err
	}

	return *data.SetVMSize.VMSize, nil
}

func (c *Client) GetAppVMCount(appID string) ([]TaskGroupCount, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				id
				name
				taskGroupCounts {
					name
					count
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appID)

	data, err := c.Run(req)
	if err != nil {
		return []TaskGroupCount{}, err
	}

	return data.App.TaskGroupCounts, nil
}

func (c *Client) SetAppVMCount(appID string, count int, maxPerRegion int) ([]TaskGroupCount, []string, error) {
	query := `
		mutation ($input: SetVMCountInput!) {
			setVmCount(input: $input) {
				taskGroupCounts {
					name
					count
				}
				warnings
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", SetVMCountInput{
		AppID: appID,
		GroupCounts: []VMCountInput{
			{Group: "app", Count: count, MaxPerRegion: maxPerRegion},
		}})

	data, err := c.Run(req)
	if err != nil {
		return []TaskGroupCount{}, []string{}, err
	}

	return data.SetVMCount.TaskGroupCounts, data.SetVMCount.Warnings, nil
}
