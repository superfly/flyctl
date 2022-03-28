package api

import (
	"context"
)

func (client *Client) ListMachines(ctx context.Context, appID string, state string) ([]*Machine, error) {
	query := `
		query($state: String, $appId: String) {
			machines(state: $state, appId: $appId) {
				nodes {
					id
					name
					config
					state
					region
					createdAt
					app {
						name
					}
					ips {
						nodes {
							family
							kind
							ip
							maskSize
						}
					}
					host {
						id
					}
				}
			}
		}
		`

	req := client.NewRequest(query)

	if state != "" {
		req.Var("state", state)
	}

	if appID != "" {
		req.Var("appId", appID)
	}

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.Machines.Nodes, nil
}

func (client *Client) LaunchMachine(ctx context.Context, input LaunchMachineInput) (*Machine, *App, error) {
	query := `
			mutation($input: LaunchMachineInput!) {
				launchMachine(input: $input) {
					machine {
						id
						state
						ips {
							nodes {
								family
								kind
								ip
								maskSize
							}
						}
						host {
							id
						}
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

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	return data.LaunchMachine.Machine, data.LaunchMachine.App, nil
}

func (client *Client) StopMachine(ctx context.Context, input StopMachineInput) (*Machine, error) {
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

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.StopMachine.Machine, nil
}

func (client *Client) StartMachine(ctx context.Context, input StartMachineInput) (*Machine, error) {
	query := `
	mutation($input: StartMachineInput!) {
		startMachine(input: $input) {
			machine {
				id
				state
			}
		}
	}
	`

	req := client.NewRequest(query)

	req.Var("input", input)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.StartMachine.Machine, nil
}

func (client *Client) KillMachine(ctx context.Context, input KillMachineInput) (*Machine, error) {
	query := `
	mutation($input: KillMachineInput!) {
		killMachine(input: $input) {
			machine {
				id
				state
			}
		}
	}
	`

	req := client.NewRequest(query)

	req.Var("input", input)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.KillMachine.Machine, nil
}

func (client *Client) RemoveMachine(ctx context.Context, input RemoveMachineInput) (*Machine, error) {
	query := `
	mutation($input: RemoveMachineInput!) {
		removeMachine(input: $input) {
			machine {
				id
				state
			}
		}	}
	`

	req := client.NewRequest(query)

	req.Var("input", input)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.RemoveMachine.Machine, nil
}

func (client *Client) GetMachine(ctx context.Context, appID, machineID string) (*Machine, error) {

	query := `
	query($appName: String!, $machineId: String!) {
		app(name: $appName) {
		  	machine(id: $machineId){
		  		id
		  		name
		  		region
		  		state
				config
		  		createdAt
		  		app{
					name
					hostname
		  		}
				ips{
					nodes{
					  id
					  ip
					  kind
					  family
					  maskSize  
					}
				}
				events{
					nodes{
						id
						kind
						timestamp
						...on MachineEventExit {
							metadata
						}
					}
				}
			}
		}
	}
	`

	req := client.NewRequest(query)

	if appID != "" {
		req.Var("appName", appID)
	}

	req.Var("machineId", machineID)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Machine, nil
}
