package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newLaunch() (cmd *cobra.Command) {
	const (
		// TODO: document command
		long = `
`

		// TODO: document command
		short = ""
		usage = "launch [-o ORG] [-r REGION] [NAME]"
	)

	cmd = command.New(usage, short, long, runLaunch,
		command.RequireSession)

	cmd.Args = cobra.RangeArgs(0, 1)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
	)

	return
}

func runLaunch(ctx context.Context) (err error) {
	name := flag.FirstArg(ctx)
	_ = name

	var org *api.Organization
	if org, err = prompt.Org(ctx, nil); err != nil {
		return
	}

	var region *api.Region
	if region, err = prompt.Region(ctx); err != nil {
		return
	}

	var password string
	if password, err = helpers.RandString(30); err != nil {
		return
	}

	client := client.FromContext(ctx).API()

	imageRef, err := client.GetLatestImageTag(ctx, "flyio/redis")
	if err != nil {
		return err
	}

	input := api.LaunchMachineInput{
		Name:    flag.FirstArg(ctx),
		OrgSlug: org.ID,
		Region:  region.Code,
		Config: &api.MachineConfig{
			Image: imageRef,
			Env: map[string]string{
				"REDIS_PASSWORD": password,
			},
		},
	}

	var machine *api.Machine
	var app *api.App

	if machine, app, err = client.LaunchMachine(ctx, input); err != nil {
		err = fmt.Errorf("failed launching machine: %w", err)

		return
	}

	if io := iostreams.FromContext(ctx); config.FromContext(ctx).JSONOutput {
		_ = render.JSON(io.Out, struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}{
			ID:   machine.ID,
			Name: machine.Name,
		})
	} else {
		fmt.Fprintf(io.Out, "machine %s (%s) created.\n", machine.Name, machine.ID)
		fmt.Fprintf(io.Out, "Access your Redis instance at redis://:%s@top1.nearest.of.%s.internal:6379", password, app.Name)
	}

	return
}
