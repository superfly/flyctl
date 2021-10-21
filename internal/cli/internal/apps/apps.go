// Package apps implements the apps command chain.
package apps

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/internal/cli/internal/cmd"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

var errNotImplementedYet = errors.New("not implemented yet")

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	apps := cmd.New("apps", nil)

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
	return cmd.New("apps.list", runList,
		cmd.RequireSession)
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
	create := cmd.New("apps.create", runCreate,
		cmd.RequireSession)

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
	destroy := cmd.New("apps.destroy", runDestroy,
		cmd.RequireSession)

	destroy.Args = cobra.ExactArgs(1)

	flag.Add(destroy, nil,
		flag.Yes())

	return destroy
}

func runDestroy(ctx context.Context) error {
	return errNotImplementedYet
}

func newMove() *cobra.Command {
	move := cmd.New("apps.move", runMove,
		cmd.RequireSession)

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
	suspend := cmd.New("apps.suspend", runSuspend,
		cmd.RequireSession, requireAppNameAsArg)

	suspend.Args = cobra.RangeArgs(0, 1)

	return suspend
}

func runSuspend(ctx context.Context) error {
	return errNotImplementedYet
}

func newResume() *cobra.Command {
	resume := cmd.New("apps.resume", runResume,
		cmd.RequireSession, requireAppNameAsArg)

	resume.Args = cobra.RangeArgs(0, 1)

	return resume
}

func runResume(ctx context.Context) error {
	return errNotImplementedYet
}

func newRestart() *cobra.Command {
	restart := cmd.New("apps.restart", runRestart,
		cmd.RequireSession, requireAppNameAsArg)

	restart.Args = cobra.RangeArgs(0, 1)

	return restart
}

func runRestart(ctx context.Context) error {
	return errNotImplementedYet
}

func requireAppNameAsArg(context.Context) (context.Context, error) {
	return nil, errNotImplementedYet
}
