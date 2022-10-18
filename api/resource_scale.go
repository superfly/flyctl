package api

import "context"

func (c *Client) ScaleApp(ctx context.Context, appID string, regions []ScaleRegionInput) ([]ScaleRegionChange, error) {
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

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.ScaleApp.Delta, nil
}

func (c *Client) UpdateAutoscaleConfig(ctx context.Context, input UpdateAutoscaleConfigInput) (*AutoscalingConfig, error) {
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

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.UpdateAutoscaleConfig.App.Autoscaling, nil
}

func (c *Client) AppAutoscalingConfig(ctx context.Context, appName string) (*AutoscalingConfig, error) {
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

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Autoscaling, nil
}

func (c *Client) AppVMResources(ctx context.Context, appName string) (VMSize, []TaskGroupCount, []ProcessGroup, error) {
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
				processGroups {
					name
					vmSize {
						name
						cpuCores
						memoryGb
						memoryMb
						priceMonth
						priceSecond
					}
					maxPerRegion
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return VMSize{}, []TaskGroupCount{}, []ProcessGroup{}, err
	}

	return data.App.VMSize, data.App.TaskGroupCounts, data.App.ProcessGroups, nil
}

func (c *Client) SetAppVMSize(ctx context.Context, appID string, group string, sizeName string, memoryMb int64) (VMSize, error) {
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
				processGroup{
					name
					vmSize{
						name
						cpuCores
						memoryGb
						memoryMb
						priceMonth
						priceSecond
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", SetVMSizeInput{
		AppID:    appID,
		Group:    group,
		SizeName: sizeName,
		MemoryMb: memoryMb,
	})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return VMSize{}, err
	}

	processGroup := data.SetVMSize.ProcessGroup

	if processGroup != nil && processGroup.VMSize != nil {
		return *processGroup.VMSize, nil
	}
	return *data.SetVMSize.VMSize, nil
}

func (c *Client) GetAppVMCount(ctx context.Context, appID string) ([]TaskGroupCount, error) {
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

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return []TaskGroupCount{}, err
	}

	return data.App.TaskGroupCounts, nil
}

func (c *Client) SetAppVMCount(ctx context.Context, appID string, counts map[string]int, maxPerRegion *int) ([]TaskGroupCount, []string, error) {
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

	groups := []VMCountInput{}

	for name, count := range counts {
		g := VMCountInput{
			Group:        name,
			Count:        count,
			MaxPerRegion: maxPerRegion,
		}
		groups = append(groups, g)
	}

	req.Var("input", SetVMCountInput{
		AppID:       appID,
		GroupCounts: groups,
	})

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return []TaskGroupCount{}, []string{}, err
	}

	return data.SetVMCount.TaskGroupCounts, data.SetVMCount.Warnings, nil
}
