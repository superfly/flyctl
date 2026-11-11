package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"
)

func New() *cobra.Command {
	var (
		long = strings.Trim(`Forward a local port to a port in your Fly.io private network through a
WireGuard tunnel, and keep forwarding until you stop the command. The first
argument is local:remote, or a single port to use for both, for example
`+"`fly proxy 5432`"+` or `+"`fly proxy 15432:5432`"+`.

The remote host defaults to <app>.internal for the app from `+"`-a`"+` or
fly.toml, which reaches one of the app's Machines; pass a host name or 6PN
address as the second argument to connect elsewhere, or use `+"`--select`"+`
to choose a Machine from a list. With no app and no `+"`-o`"+`, you are
prompted for an organization.

The local listener binds to 127.0.0.1 unless you change it with
`+"`--bind-addr`"+`. Starts the Fly agent if it isn't running.`, "\n")
		short = `Proxy a local port to a Fly Machine or private network host`
	)

	cmd := command.New("proxy <local:remote> [remote_host]", short, long, run,
		command.RequireSession, command.LoadAppNameIfPresent)

	cmd.Args = cobra.RangeArgs(1, 2)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Default:     false,
			Description: "Prompt to select from available Machines from the current application",
		},
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Don't print progress indicators for WireGuard",
		},
		flag.String{
			Name:        flagnames.BindAddr,
			Shorthand:   "b",
			Default:     "127.0.0.1",
			Description: "Local address to bind to",
		},
		flag.Bool{
			Name:        "watch-stdin",
			Default:     false,
			Description: "Watches stdin and terminates once it gets closed",
		},
	)

	return cmd
}

func run(ctx context.Context) (err error) {
	client := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	orgSlug := flag.GetOrg(ctx)

	args := flag.Args(ctx)
	promptInstance := flag.GetBool(ctx, "select")

	if promptInstance && appName == "" {
		return errors.New("--app required when --select flag provided")
	}

	if orgSlug != "" {
		if _, err := uiexutil.ClientFromContext(ctx).GetOrganization(ctx, orgSlug); err != nil {
			return err
		}
	}

	if appName == "" && orgSlug == "" {
		org, err := prompt.Org(ctx)
		if err != nil {
			return err
		}
		orgSlug = org.Slug
	}

	var network string
	if appName != "" {
		app, err := flapsutil.ClientFromContext(ctx).GetApp(ctx, appName)
		if err != nil {
			return err
		}
		orgSlug = app.Organization.Slug
		network = flapsutil.NetworkName(app)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return err
	}

	// do this explicitly so we can get the DNS server address
	_, err = agentclient.Establish(ctx, orgSlug, network)
	if err != nil {
		return err
	}

	dialer, err := agentclient.ConnectToTunnel(ctx, orgSlug, network, flag.GetBool(ctx, "quiet"))
	if err != nil {
		return err
	}

	ports := strings.Split(args[0], ":")

	params := &proxy.ConnectParams{
		BindAddr:         flag.GetBindAddr(ctx),
		Ports:            ports,
		AppName:          appName,
		OrganizationSlug: orgSlug,
		Dialer:           dialer,
		PromptInstance:   promptInstance,
		Network:          network,
	}

	if len(args) > 1 {
		params.RemoteHost = args[1]
	} else {
		params.RemoteHost = fmt.Sprintf("%s.internal", appName)
	}

	if flag.GetBool(ctx, "watch-stdin") {
		ctx = watchStdinAndAbortOnClose(ctx)
	}

	return proxy.Connect(ctx, params)
}

// Asynchronously watches stdin and abort when it closes.
//
// There is no guarantee that a OS process spawning the proxy will
// terminate it, however the stdin is always closed whtn the parent
// terminates. This way we make sure there are no zombie processes,
// especially that they hold onto TCP ports.
func watchStdinAndAbortOnClose(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)

	ios := iostreams.FromContext(ctx)

	go func() {
		// We don't expect any input, but if there is one, we ignore it
		// to avoid allocating space unnecessarily
		buffer := make([]byte, 1)
		for {
			_, err := ios.In.Read(buffer)
			if err == io.EOF {
				cancel(nil)
			} else if err != nil {
				cancel(err)
			}
		}
	}()

	return ctx
}
