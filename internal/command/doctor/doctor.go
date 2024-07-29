// Package doctor implements the doctor command chain.
package doctor

import (
	"context"
	"fmt"

	dockerclient "github.com/docker/docker/client"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/flag/completion"
	"github.com/superfly/flyctl/internal/flyutil"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/doctor/diag"
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
		flag.JSONOutput(),
		flag.Bool{
			Name:        "verbose",
			Shorthand:   "v",
			Default:     false,
			Description: "Print extra diagnostic information.",
		},
		flag.String{
			Name:         "org",
			Shorthand:    "o",
			Description:  "The name of the organization to use for WireGuard tests.",
			Default:      "personal",
			CompletionFn: completion.CompleteOrgs,
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

	// This JSON output is (unfortunately) depended on in production.
	// Adding to it is perfectly safe, but double-check WGCI before changing or removing anything :)
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

	wgOrgSlug := flag.GetString(ctx, "org")

	lprint(nil, "Pinging WireGuard gateway (give us a sec)... ")
	err = runPersonalOrgPing(ctx, wgOrgSlug)
	if !check("ping", err) {
		lprint(nil, `
We can't establish connectivity with WireGuard for your personal organization.

WireGuard runs on 51820/udp, which your local network may block.

If this is the first time you've ever used 'flyctl' on this machine, you
can try running 'flyctl doctor' again.

If this was working before, you can ask 'flyctl' to create a new peer for
you by running 'flyctl wireguard reset'.

If your network might be blocking UDP, you can run 'flyctl wireguard websockets enable',
followed by 'flyctl agent restart', and we'll run WireGuard over HTTPS.
`)
		return nil
	}

	// ------------------------------------------------------------
	// Check if we can access DNS and Flaps via WireGuard
	// ------------------------------------------------------------

	lprint(nil, "Testing WireGuard DNS... ")
	err = runPersonalOrgCheckDns(ctx, wgOrgSlug)
	if !check("wgdns", err) {
		lprint(nil, `
We can't resolve internal DNS for your personal organization.
This is likely a platform issue, please contact support.
`)
		return nil
	}

	lprint(nil, "Testing WireGuard Flaps... ")
	err = runPersonalOrgCheckFlaps(ctx, wgOrgSlug)
	if !check("wgflaps", err) {
		lprint(nil, `
We can't access Flaps via a WireGuard tunnel into your personal organization.
This is likely a platform issue, please contact support.
`)
		return nil
	}

	// ------------------------------------------------------------
	// App specific checks below here
	// ------------------------------------------------------------

	appChecker, err := NewAppChecker(ctx, isJson, color)
	if err != nil {
		return err
	}
	if appChecker == nil {
		return nil
	}
	appChecks := appChecker.checkAll()
	for k, v := range appChecks {
		checks[k] = v
	}

	// ------------------------------------------------------------

	return nil
}

func runAuth(ctx context.Context) (err error) {
	client := flyutil.ClientFromContext(ctx)

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
