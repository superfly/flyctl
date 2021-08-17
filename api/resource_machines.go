package api

func (client *Client) ListMachines(state string) ([]*Machine, error) {
	query := `
		query($state: String) {
			machines(state: $state) {
				nodes {
					id
					state
					app {
						name
					}
				}
			}
		}
		`

	req := client.NewRequest(query)

	req.Var("state", state)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return data.Machines.Nodes, nil
}

func (client *Client) LaunchMachine(input LaunchMachineInput) (*Machine, *App, error) {
	query := `
			mutation($input: LaunchMachineInput!) {
				launchMachine(input: $input) {
					machine {
						id
						state
					}
					app {
						name
						organization {
							id
							slug
							name
						}
					}
				}
			}
		`

	req := client.NewRequest(query)

	req.Var("input", input)

	data, err := client.Run(req)
	if err != nil {
		return nil, nil, err
	}

	return data.LaunchMachine.Machine, data.LaunchMachine.App, nil
}

func (client *Client) StopMachine(input StopMachineInput) (*Machine, error) {
	query := `
	mutation($input: StopMachineInput!) {
		stopMachine(input: $input) {
			machine {
				id
				state
			}
		}
	}
	`

	req := client.NewRequest(query)

	req.Var("input", input)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return data.StopMachine.Machine, nil
}
