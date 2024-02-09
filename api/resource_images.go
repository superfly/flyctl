package api

import "context"

func (client *Client) GetLatestImageTag(ctx context.Context, repository string, snapshotId *string) (string, error) {
	query := `
		query($repository: String!, $snapshotId: ID) {
			latestImageTag(repository: $repository, snapshotId: $snapshotId)
		}
	`
	req := client.NewRequest(query)
	req.Var("repository", repository)
	req.Var("snapshotId", snapshotId)
	ctx = ctxWithAction(ctx, "get_latest_image_tag")

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
	ctx = ctxWithAction(ctx, "get_latest_image_details")
	req.Var("image", image)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	return &data.LatestImageDetails, nil
}
