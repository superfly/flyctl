package recipe

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/hashicorp/go-version"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
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

	io := iostreams.FromContext(ctx)
	flapsClient := flaps.FromContext(ctx)
	dialer := agent.DialerFromContext(ctx)
	client := client.FromContext(ctx).API()

	fmt.Fprintf(io.Out, "Processing Recipe: %q\n", r.Name)

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	// Evaluate whether we require a lease.
	if r.RequireLease {
		fmt.Fprintln(io.Out, "Acquiring lease")

		for _, machine := range machines {
			lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(40))
			if err != nil {
				return fmt.Errorf("failed to obtain lease: %w", err)
			}
			machine.LeaseNonce = lease.Data.Nonce

			// Ensure lease is released on return
			defer flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
		}

		// Requery machines after lease acquisition so we can ensure we are evaluating the most
		// up-to-date configuration.
		machines, err = flapsClient.ListActive(ctx)
		if err != nil {
			return fmt.Errorf("machines could not be retrieved %w", err)
		}
	}

	// Verify constraints
	fmt.Fprintln(io.Out, "Verifying constraints")
	if err = r.verifyConstraints(machines); err != nil {
		return err
	}

	// Evaluate selectors that require pre-processing.
	for _, op := range r.Operations {
		if op.Selector.Preprocess {
			op.Targets = op.ProcessSelectors(machines)
		}
	}

	// Evaluate operations
	for _, op := range r.Operations {
		// Process selectors if they were not pre-processed.
		if !op.Selector.Preprocess {
			op.Targets = op.ProcessSelectors(machines)
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
			fmt.Fprintf(io.Out, "Performing %s command %q\n", op.Type, op.Name)

			err := ssh.SSHConnect(&ssh.SSHParams{
				Ctx:    ctx,
				Org:    r.App.Organization,
				Dialer: dialer,
				App:    r.App.Name,
				Cmd:    op.SSHConnectCommand.Command,
				Stdin:  os.Stdin,
				Stdout: os.Stdout,
				Stderr: os.Stderr,
			}, op.Targets[0].PrivateIP)
			if err != nil {
				return err
			}

			continue
		// GraphQL
		case CommandTypeGraphql:
			fmt.Fprintf(io.Out, "Performing %s command %q\n", op.Type, op.Name)

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

		case CommandTypeWaitFor:
			fmt.Fprintf(io.Out, "Performing %s command %q\n", op.Type, op.Name)

			if op.WaitForCommand.HealthCheck == (HealthCheckSelector{}) {
				continue
			}

			retries := op.WaitForCommand.Retries
			if retries != 0 {
				retries = 30 // default
			}

			interval := op.WaitForCommand.Interval
			if interval.String() == "0s" {
				interval = time.Second // default
			}

			// TODO - This should work with multiple targets
			machine := op.Targets[0]

			if err := retry.Do(
				func() error {
					machine, err = flapsClient.Get(ctx, machine.ID)
					if err != nil {
						return err
					}
					if matchesHealthCheckConstraints(machine, op.WaitForCommand.HealthCheck) {
						return fmt.Errorf("%s does not meet condition yet: %v", machine.ID, op.WaitForCommand.HealthCheck)
					}

					return nil
				},
				retry.Context(ctx), retry.Attempts(uint(retries)), retry.Delay(interval), retry.DelayType(retry.FixedDelay),
			); err != nil {
				return err
			}

			continue

		case CommandTypeCustom:
			fmt.Fprintf(io.Out, "Performing %s command %q\n", op.Type, op.Name)

			if err := op.CustomCommand(); err != nil {
				return err
			}

			continue
		}

		for _, machine := range op.Targets {
			fmt.Fprintf(io.Out, "Performing %s command %q against: %s\n", op.Type, op.Name, machine.ID)

			switch op.Type {
			// Flaps
			case CommandTypeFlaps:
				var argArr []string
				for key, value := range op.FlapsCommand.Options {
					argArr = append(argArr, fmt.Sprintf("%s=%s", key, value))
				}

				options := strings.Join(argArr, "&")
				path := fmt.Sprintf("/%s/%s?%s", machine.ID, op.FlapsCommand.Action, options)
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

	fmt.Fprintf(io.Out, "%q finished successfully!\n", r.Name)

	return nil
}

func (r *RecipeTemplate) verifyConstraints(machines []*api.Machine) error {

	if r.Constraints.AppRoleID != "" && r.Constraints.AppRoleID != r.App.PostgresAppRole.Name {
		return fmt.Errorf("Recipe %s is not compatible with app %s", r.Name, r.App.Name)
	}

	if r.Constraints.PlatformVersion != "" && r.App.PlatformVersion != r.Constraints.PlatformVersion {
		return fmt.Errorf("Recipe %s is not compatible with apps running on %s", r.Name, r.App.PlatformVersion)
	}

	if len(r.Constraints.Images) != 0 {
		var verfiedMachines []*api.Machine
		for _, m := range machines {
			valid := false

			// If there are multiple image requirements present only one
			// of the image requirements need to be met.
			for _, img := range r.Constraints.Images {

				if img.Registry != "" {
					if img.Registry != m.ImageRef.Registry {
						continue
					}
				}

				if img.Repository != "" {
					if img.Repository != m.ImageRef.Repository {
						continue
					}
				}

				if img.MinFlyVersion != "" {
					requiredVersion, err := version.NewVersion(img.MinFlyVersion)
					if err != nil {
						return err
					}

					imageVersionStr := m.ImageVersion()[1:]
					imageVersion, err := version.NewVersion(imageVersionStr)
					if err != nil {
						return err
					}

					if imageVersion.LessThan(requiredVersion) {
						continue
					}
				}
				valid = true
			}

			if valid {
				verfiedMachines = append(verfiedMachines, m)
			}
		}

		// Expand on the output provided here so it's clear to the end user
		// which requirements were not met.
		if len(verfiedMachines) != len(machines) {
			return fmt.Errorf("Image requirements were not met")
		}

	}

	return nil
}
