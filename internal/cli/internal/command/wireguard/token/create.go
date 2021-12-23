package token

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

func newCreate() (cmd *cobra.Command) {
	const (
		short = "Create a new WireGuard token"
		long  = short + "\n"
		usage = "create [-org ORG] [NAME]"
	)

	cmd = command.New(usage, short, long, runCreate,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.Org(),
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return
}

func runCreate(ctx context.Context) error {
	org, err := prompt.Org(ctx, nil)
	if err != nil {
		return err
	}

	name, err := nameFromFirstArgOrPrompt(ctx)
	if err != nil {
		return err
	}

	client := client.FromContext(ctx).API()

	data, err := client.CreateDelegatedWireGuardToken(ctx, org, name)
	if err != nil {
		return fmt.Errorf("failed creating WireGuard token: %w", err)
	}

	io := iostreams.FromContext(ctx)

	if config.FromContext(ctx).JSONOutput {
		_ = render.JSON(io.Out, struct {
			Token string `json:"token"`
		}{
			Token: data.Token,
		})

		return nil
	}

	fmt.Fprintln(io.ErrOut, `
!!!! WARNING: Output includes credential information. Credentials cannot !!!!
!!!! be recovered after creation; if you lose the token, you'll need to  !!!!
!!!! remove and and re-add it.                                           !!!!

To use a token to create a WireGuard connection, you can use curl:

    curl -v --request POST
         -H "Authorization: Bearer ${WG_TOKEN}" \
         -H "Content-Type: application/json"    \
         --data '{"name": "node-1",             \
                  "group": "k8s",               \
                  "pubkey": "'"${WG_PUBKEY}"'", \
                  "region": "dev"}'             \
         http://fly.io/api/v3/wire_guard_peers

We'll return 'us' (our local 6PN address), 'them' (the gateway IP address),
and 'pubkey' (the public key of the gateway), which you can inject into a
"wg.con".`)

	fmt.Fprintf(io.Out, "token created: %s\n", data.Token)

	return nil
}
