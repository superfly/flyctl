package api

func (client *Client) RestartAllocation(appName string, allocId string) error {
	query := `
		mutation($input: RestartAllocationInput!) {
			restartAllocation(input: $input) {
				app {
					name
				}
				allocation {
					id
				}
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", map[string]string{
		"appId":   appName,
		"allocId": allocId,
	})

	_, err := client.Run(req)
	return err
}

func (client *Client) StopAllocation(appName string, allocId string) error {
	query := `
		mutation($input: StopAllocationInput!) {
			stopAllocation(input: $input) {
				app {
					name
				}
				allocation {
					id
				}
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", map[string]string{
		"appId":   appName,
		"allocId": allocId,
	})

	_, err := client.Run(req)
	return err
}
