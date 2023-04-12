package logs

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newUnship() (cmd *cobra.Command) {
	const (
		short = "Stop shipping application logs to Logtail"
		long  = short + "\n"
	)

	cmd = command.New("unship", short, long, runUnship, command.RequireSession, command.RequireAppName)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runUnship(ctx context.Context) (err error) {

	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API().GenqClient
		io     = iostreams.FromContext(ctx)
	)

	appName := appconfig.NameFromContext(ctx)
	appNameResponse, err := gql.GetApp(ctx, client, appName)

	if err != nil {
		return err
	}

	targetApp := appNameResponse.App.AppData
	targetOrg := targetApp.Organization

	_, err = gql.DeleteAddOn(ctx, client, appName+"-log-shipper")

	if err != nil {
		return
	}

	flapsClient, machine, err := EnsureShipperMachine(ctx, targetOrg)

	if err != nil {
		return
	}

	cmd := []string{"/remove-logger.sh", targetApp.Name, "logtail"}

	request := &api.MachineExecRequest{
		Cmd: strings.Join(cmd, " "),
	}

	response, err := flapsClient.Exec(ctx, machine.ID, request)

	if err != nil {
		fmt.Fprintf(io.ErrOut, response.StdErr)
		return err
	}
	fmt.Fprintf(out, "Logs for %s are no longer being shipped, but older logs are still preserved in Logtail.\n", appName)
	return
}
