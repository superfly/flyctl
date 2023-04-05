package api

import (
	"context"
)

func (c *Client) ConfigureRegions(ctx context.Context, input ConfigureRegionsInput) ([]Region, []Region, error) {
	query := `
		mutation ($input: ConfigureRegionsInput!) {
			configureRegions(input: $input) {
				regions {
					code
					name
				}
				backupRegions{
					code
					name
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", input)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	return data.ConfigureRegions.Regions, data.ConfigureRegions.BackupRegions, nil
}

type RegionList struct {
	PlatformVersion string
	Regions         []Region
	BackupRegions   []Region
	ProcessGroups   []ProcessGroup
	MachineRegions  map[string][]string
}

func (c *Client) ListAppRegions(ctx context.Context, appName string) (RegionList, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				regions{
					code
					name
				}
				backupRegions{
					code
					name
				}
				processGroups{
					name
					regions
				}
				platformVersion
				machines {
					nodes {
						config
						region
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return RegionList{}, err
	}

	if data.App.PlatformVersion == "nomad" {
		return RegionList{
			PlatformVersion: data.App.PlatformVersion,
			Regions:         *data.App.Regions,
			BackupRegions:   *data.App.BackupRegions,
			ProcessGroups:   data.App.ProcessGroups,
		}, nil
	}

	regionList := RegionList{
		PlatformVersion: data.App.PlatformVersion,
		MachineRegions:  make(map[string][]string),
	}

	for _, node := range data.App.Machines.Nodes {
		regionList.MachineRegions[node.Config.ProcessGroup()] = append(regionList.MachineRegions[node.Config.ProcessGroup()], node.Region)
	}

	return regionList, nil
}

func (c *Client) GetNearestRegion(ctx context.Context) (*Region, error) {
	req := c.NewRequest(`
		query {
			nearestRegion {
				code
				name
				gatewayAvailable
			}
		}
`)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.NearestRegion, nil
}
