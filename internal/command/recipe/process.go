package recipe

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/watch"
)

func (r *RecipeTemplate) Setup(ctx context.Context) (context.Context, error) {
	client := client.FromContext(ctx).API()

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, r.App.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("can't build tunnel for %s: %s", r.App.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flaps.New(ctx, r.App)
	if err != nil {
		return nil, fmt.Errorf("Unable to establish connection with flaps: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	return ctx, nil
}

func (r *RecipeTemplate) Process(ctx context.Context) error {
	ctx, err := r.Setup(ctx)
	if err != nil {
		return err
	}

	flapsClient := flaps.FromContext(ctx)
	dialer := agent.DialerFromContext(ctx)
	client := client.FromContext(ctx).API()

	// Evaluate whether we require a lease.
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

	// Evaluate operations
	for _, op := range r.Operations {
		targetMachines := machines

		// Evaluate selectors if provided
		if op.Selector.HealthCheck != (HealthCheckSelector{}) {
			var newTargets []*api.Machine
			for _, m := range targetMachines {
				for _, check := range m.Checks {
					if check.Name == op.Selector.HealthCheck.Name && check.Output == op.Selector.HealthCheck.Value {
						newTargets = append(newTargets, m)
					}
				}
			}
			targetMachines = newTargets
		}

		if op.Prompt != (PromptDefinition{}) {
			confirm := false
			confirm, err := prompt.Confirm(ctx, op.Prompt.Message)
			if err != nil {
				return err
			}
			if !confirm {
				return fmt.Errorf("I guess we are done here")
			}
		}

		switch op.Type {
		// SSH Connect
		case CommandTypeSSHConnect:
			err := ssh.SSHConnect(&ssh.SSHParams{
				Ctx:    ctx,
				Org:    r.App.Organization,
				Dialer: dialer,
				App:    r.App.Name,
				Cmd:    op.SSHConnectCommand.Command,
				Stdin:  os.Stdin,
				Stdout: os.Stdout,
				Stderr: os.Stderr,
			}, targetMachines[0].PrivateIP)
			if err != nil {
				return err
			}

			continue
		// GraphQL
		case CommandTypeGraphql:
			fmt.Printf("Performing %s command %q\n", op.Type, op.Name)
			req := client.NewRequest(op.GraphQLCommand.Query)

			for key, value := range op.GraphQLCommand.Variables {
				req.Var(key, value)
			}

			data, err := client.RunWithContext(ctx, req)
			if err != nil {
				return err
			}

			op.GraphQLCommand.Result = &data

			continue

		case CommandTypeCustom:
			fmt.Printf("Performing %s command %q\n", op.Type, op.Name)

			if err := op.CustomCommand(); err != nil {
				return err
			}

			continue
		}

		for _, machine := range targetMachines {
			fmt.Printf("Performing %s command %q against: %s\n", op.Type, op.Name, machine.ID)

			switch op.Type {
			// Flaps
			case CommandTypeFlaps:
				var argArr []string
				for key, value := range op.FlapsCommand.Options {
					argArr = append(argArr, fmt.Sprintf("%s=%s", key, value))
				}

				options := strings.Join(argArr, "&")
				path := fmt.Sprintf("/%s/%s?%s", machine.ID, op.FlapsCommand.Action, options)
				fmt.Printf("Path: %s\n", path)
				if err = flapsClient.SendRequest(ctx, op.FlapsCommand.Method, path, nil, nil, nil); err != nil {
					return err
				}

			// HTTP
			case CommandTypeHTTP:
				cmd := op.HTTPCommand
				client := NewFromInstance(machine.PrivateIP, cmd.Port, dialer)

				if err := client.Do(ctx, cmd.Method, cmd.Endpoint, cmd.Data, cmd.Result); err != nil {
					return err
				}

			// SSH Connect
			case CommandTypeSSHCommand:
				_, err := ssh.RunSSHCommand(ctx, r.App, dialer, machine.PrivateIP, op.SSHRunCommand.Command)
				if err != nil {
					return err
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
