package api

import "context"

func (client *Client) GetImageInfo(ctx context.Context, appName string) (*App, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				imageVersionTrackingEnabled
				imageUpgradeAvailable
				imageDetails {
					registry
					repository
					tag
					digest
					version
				}
				latestImageDetails {
					registry
					repository
					tag
					digest
					version
				}
				machines{
					nodes{
					  id
					  name
					  config
					}
				  }
			}
		}
	`
	req := client.NewRequest(query)
	req.Var("appName", appName)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.App, nil
}

func (client *Client) GetLatestImageTag(ctx context.Context, repository string) (string, error) {
	query := `
		query($repository: String!) {
			latestImageTag(repository: $repository) 
		}
	`
	req := client.NewRequest(query)
	req.Var("repository", repository)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return "", err
	}

	return data.LatestImageTag, nil
}
