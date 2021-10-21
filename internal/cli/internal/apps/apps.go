// Package apps implements the apps command chain.
package apps

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

var errNotImplementedYet = errors.New("not implemented yet")

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	apps := command.New("apps", nil)

	apps.AddCommand(
		newList(),
		newCreate(),
		newDestroy(),
		newMove(),
		newSuspend(),
		newResume(),
		newRestart(),
	)

	return apps
}

func newList() *cobra.Command {
	return command.New("apps.list", runList,
		command.RequireSession)
}

func runList(ctx context.Context) error {
	client := client.FromContext(ctx)

	apps, err := client.API().GetApps(ctx, nil)
	if err != nil {
		return err
	}

	p := &presenters.Apps{
		Apps: apps,
	}

	opt := presenters.Options{
		AsJSON: false,
	}

	return render.Presentable(ctx, p, opt)
}

func newCreate() *cobra.Command {
	create := command.New("apps.create", runCreate,
		command.RequireSession)

	create.Args = cobra.RangeArgs(0, 1)

	flag.Add(create, nil,
		flag.Org(),
		flag.String{
			Name:        "name",
			Description: "The app name to use",
		},
		flag.Bool{
			Name:        "generate-name",
			Description: "Always generate a name for the app",
		},
	)

	return create
}

func runCreate(ctx context.Context) error {
	return errNotImplementedYet
}

func newDestroy() *cobra.Command {
	destroy := command.New("apps.destroy", runDestroy,
		command.RequireSession)

	destroy.Args = cobra.ExactArgs(1)

	flag.Add(destroy, nil,
		flag.Yes())

	return destroy
}

func runDestroy(ctx context.Context) error {
	return errNotImplementedYet
}

func newMove() *cobra.Command {
	move := command.New("apps.move", runMove,
		command.RequireSession)

	move.Args = cobra.ExactArgs(1)

	flag.Add(move, nil,
		flag.Yes(),
		flag.Org(),
	)

	return move
}

func runMove(ctx context.Context) error {
	return errNotImplementedYet
}

func newSuspend() *cobra.Command {
	suspend := command.New("apps.suspend", runSuspend,
		command.RequireSession, requireAppNameAsArg)

	suspend.Args = cobra.RangeArgs(0, 1)

	return suspend
}

func runSuspend(ctx context.Context) error {
	return errNotImplementedYet
}

func newResume() *cobra.Command {
	resume := command.New("apps.resume", runResume,
		command.RequireSession, requireAppNameAsArg)

	resume.Args = cobra.RangeArgs(0, 1)

	return resume
}

func runResume(ctx context.Context) error {
	return errNotImplementedYet
}

func newRestart() *cobra.Command {
	restart := command.New("apps.restart", runRestart,
		command.RequireSession, requireAppNameAsArg)

	restart.Args = cobra.RangeArgs(0, 1)

	return restart
}

func runRestart(ctx context.Context) error {
	return errNotImplementedYet
}

func requireAppNameAsArg(context.Context) (context.Context, error) {
	return nil, errNotImplementedYet
}
