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

func (client *Client) GetLatestImageTag(ctx context.Context, repository string, snapshotId *string) (string, error) {
	query := `
		query($repository: String!, $snapshotId: ID) {
			latestImageTag(repository: $repository, snapshotId: $snapshotId)
		}
	`
	req := client.NewRequest(query)
	req.Var("repository", repository)
	req.Var("snapshotId", snapshotId)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return "", err
	}

	return data.LatestImageTag, nil
}

func (client *Client) GetLatestImageDetails(ctx context.Context, image string) (*ImageVersion, error) {

	query := `
		query($image: String!) {
			latestImageDetails(image: $image){
			  registry
			  repository
			  tag
			  version
			  digest
			}
		}
	`

	req := client.NewRequest(query)

	req.Var("image", image)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	return &data.LatestImageDetails, nil
}
