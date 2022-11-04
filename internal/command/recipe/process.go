package recipe

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/watch"
)

func (r *RecipeTemplate) Process(ctx context.Context) error {
	var (
		client = client.FromContext(ctx).API()
	)

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, r.App.Organization.Slug)
	if err != nil {
		return fmt.Errorf("can't build tunnel for %s: %s", r.App.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flaps.New(ctx, r.App)
	if err != nil {
		return fmt.Errorf("Unable to establish connection with flaps: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	// Evaluate whether we require a lease. If true, assume that a lease needs to be
	// acquired on all Machines.
	if r.RequireLease {
		fmt.Printf("Acquiring lease\n")
		machines, err := flapsClient.ListActive(ctx)
		if err != nil {
			return fmt.Errorf("machines could not be retrieved %w", err)
		}

		for _, machine := range machines {
			lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(40))
			if err != nil {
				return fmt.Errorf("failed to obtain lease: %w", err)
			}
			machine.LeaseNonce = lease.Data.Nonce

			// Ensure lease is released on return
			defer flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
		}

	}

	// Requery machines after lease acquisition so we can ensure we are evaluating the most
	// up-to-date configuration.
	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	// Time to evaluate the operations
	for _, op := range r.Operations {
		targetMachines := machines

		// Evaluate selectors if provided
		if op.HealthCheckSelector != (HealthCheckSelector{}) {
			var newTargets []*api.Machine
			for _, m := range targetMachines {
				for _, check := range m.Checks {
					if check.Name == op.HealthCheckSelector.Name && check.Output == op.HealthCheckSelector.Value {
						newTargets = append(newTargets, m)
					}
				}
			}
			targetMachines = newTargets
		}

		for _, machine := range targetMachines {
			fmt.Printf("Performing %s command %q against: %s\n", op.Type, op.Name, machine.ID)

			switch op.Type {
			// Machine commands
			case OperationTypeMachine:
				switch op.MachineCommand.Action {
				case "restart":
					if op.MachineCommand.Action == "restart" {
						input := api.RestartMachineInput{
							ID: machine.ID,
						}

						flapsClient.Restart(ctx, input)
					}
				}
			// HTTP commands
			case OperationTypeHTTP:
				client := NewFromInstance(machine.PrivateIP, op.HTTPCommand.Port, dialer)
				if err := client.Do(ctx, op.HTTPCommand.Method, op.HTTPCommand.Endpoint, nil, nil); err != nil {
					fmt.Printf("Error running http command: %s\n", err)
				}
			}

			if op.WaitForHealthChecks {
				// wait for health checks to pass
				if err := watch.MachinesChecks(ctx, []*api.Machine{machine}); err != nil {
					return fmt.Errorf("failed to wait for health checks to pass: %w", err)
				}
			}
		}
	}

	return nil
}
