// Package doctor implements the doctor command chain.
package doctor

import (
	"context"
	"fmt"
	"net"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/dig"
	"github.com/superfly/flyctl/internal/command/doctor/diag"
	"github.com/superfly/flyctl/internal/command/ping"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

// New initializes and returns a new doctor Command.
func New() (cmd *cobra.Command) {
	const (
		short = `The DOCTOR command allows you to debug your Fly environment`
		long  = short + "\n"
	)

	cmd = command.New("doctor", short, long, run,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "verbose",
			Shorthand:   "v",
			Default:     false,
			Description: "Print extra diagnostic information.",
		},
	)

	cmd.AddCommand(diag.New())

	return
}

func run(ctx context.Context) (err error) {
	var (
		isJson    = config.FromContext(ctx).JSONOutput
		isVerbose = flag.GetBool(ctx, "verbose")
		io        = iostreams.FromContext(ctx)
		color     = io.ColorScheme()
		checks    = map[string]string{}
	)

	lprint := func(color func(string) string, fmtstr string, args ...interface{}) {
		if isJson {
			return
		}

		if color != nil {
			fmt.Print(color(fmt.Sprintf(fmtstr, args...)))
		} else {
			fmt.Printf(fmtstr, args...)
		}
	}

	check := func(name string, err error) bool {
		if err != nil {
			lprint(color.Red, "FAILED\n(Error: %s)\n", err)
			checks[name] = err.Error()
			return false
		}

		lprint(color.Green, "PASSED\n")
		checks[name] = "ok"
		return true
	}

	defer func() {
		if isJson {
			render.JSON(iostreams.FromContext(ctx).Out, checks)
		}
	}()

	// ------------------------------------------------------------

	lprint(nil, "Testing authentication token... ")

	err = runAuth(ctx)
	if !check("auth", err) {
		lprint(nil, `
We can't authenticate you with your current authentication token.

Run 'flyctl auth login' to get a working token, or 'flyctl auth signup' if you've
never signed up before.
`)
		return nil
	}

	// ------------------------------------------------------------

	lprint(nil, "Testing flyctl agent... ")

	err = runAgent(ctx)
	if !check("agent", err) {
		lprint(nil, `
Can't communicate with flyctl's background agent.

Run 'flyctl agent restart'.
`)
		return nil
	}

	// ------------------------------------------------------------

	lprint(nil, "Testing local Docker instance... ")
	err = runLocalDocker(ctx)
	if err != nil {
		checks["docker"] = err.Error()
		if isVerbose {
			lprint(nil, `Nope
    (We got: %s)
    This is fine, we'll use a remote builder.
`, err.Error())
		} else {
			lprint(nil, "Nope\n")
		}
	} else {
		lprint(color.Green, "PASSED\n")
		checks["docker"] = "ok"
	}

	// ------------------------------------------------------------

	lprint(nil, "Pinging WireGuard gateway (give us a sec)... ")
	err = runPersonalOrgPing(ctx)
	if !check("ping", err) {
		lprint(nil, `
We can't establish connectivity with WireGuard for your personal organization.

WireGuard runs on 51820/udp, which your local network may block.

If this is the first time you've ever used 'flyctl' on this machine, you
can try running 'flyctl doctor' again.
`)
		return nil
	}

	return nil
}

func runAuth(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()

	if _, err = client.GetCurrentUser(ctx); err != nil {
		err = fmt.Errorf("can't verify access token: %w", err)
	}

	return
}

func runAgent(ctx context.Context) (err error) {
	defer func() {
		if err == nil {
			return
		}

		err = fmt.Errorf("couldn't ping agent: %w", err)
	}()

	var ac *agent.Client
	if ac, err = agent.DefaultClient(ctx); err == nil {
		_, err = ac.Ping(ctx)
	}

	return
}

func runPersonalOrgPing(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()

	ac, err := agent.DefaultClient(ctx)
	if err != nil {
		// shouldn't happen, already tested agent
		return fmt.Errorf("ping gateway: weird error: %w", err)
	}

	org, err := client.FindOrganizationBySlug(ctx, "personal")
	if err != nil {
		// shouldn't happen, already verified auth token
		return fmt.Errorf("ping gateway: weird error: %w", err)
	}

	pinger, err := ac.Pinger(ctx, "personal")
	if err != nil {
		return fmt.Errorf("ping gateway: %w", err)
	}

	defer pinger.Close()

	_, ns, err := dig.ResolverForOrg(ctx, ac, org.Slug)
	if err != nil {
		return fmt.Errorf("ping gateway: %w", err)
	}

	replyBuf := make([]byte, 1000)

	for i := 0; i < 30; i++ {
		_, err = pinger.WriteTo(ping.EchoRequest(0, i, time.Now(), 12), &net.IPAddr{IP: net.ParseIP(ns)})

		pinger.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, _, err := pinger.ReadFrom(replyBuf)
		if err != nil {
			continue
		}

		return nil
	}

	return fmt.Errorf("ping gateway: no response from gateway received")
}

func runLocalDocker(ctx context.Context) (err error) {
	defer func() {
		if err == nil {
			return
		}

		err = fmt.Errorf("failed pinging docker instance: %w", err)
	}()

	var client *dockerclient.Client
	if client, err = imgsrc.NewLocalDockerClient(); err == nil {
		_, err = client.Ping(ctx)
	}

	return
}
