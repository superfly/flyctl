// Package doctor implements the doctor command chain.
package doctor

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/miekg/dns"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
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

If this was working before, you can ask 'flyctl' to create a new peer for
you by running 'flyctl wireguard reset'.

If your network might be blocking UDP, you can run 'flyctl wireguard websockets enable',
followed by 'flyctl agent restart', and we'll run WireGuard over HTTPS.
`)
		return nil
	}

	// ------------------------------------------------------------
	// App specific checks below here
	// ------------------------------------------------------------
	appName := app.NameFromContext(ctx)
	if appName == "" {
		lprint(nil, "No app provided; skipping app specific checks\n")
		return nil
	}
	lprint(nil, "\nApp specific checks for %s:\n", appName)

	apiClient := client.FromContext(ctx).API()
	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		lprint(nil, "API error looking up app with name %s: %w\n", appName, err)
		return nil
	}

	if !app.Deployed && app.PlatformVersion != "machines" {
		lprint(color.Yellow, "%s app has not been deployed yet. Deploy using `flyctl deploy`.\n", appName)
		return nil
	}

	lprint(nil, "Checking that app has ip addresses allocated... ")

	ipAddresses, err := apiClient.GetIPAddresses(ctx, appName)
	if err != nil {
		lprint(nil, "API error listing IP addresses for app %s: %w\n", appName, err)
		return nil
	}

	if len(ipAddresses) > 0 {
		checks["appHasIps"] = "ok"
		lprint(color.Green, "PASSED\n")
	} else {
		checks["appHasIps"] = "No ips"
		lprint(nil, `Nope
	No ip addresses assigned to this app. If the app is not intended to receive traffic, this is fine.
	Otherwise, it likely means that the services configuration is not correctly setup to receive http, tls, tcp, or udp traffic.
	https://fly.io/docs/reference/configuration/#the-services-sections
`)
	}

	v4s := make(map[string]bool)
	v6s := make(map[string]bool)
	for _, ip := range ipAddresses {
		switch ip.Type {
		case "v4":
			v4s[ip.Address] = true
		case "v6":
			v6s[ip.Address] = true
		default:
			lprint(nil, "Ip address %s has unexpected type '%s'. Please file a bug with this message at https://github.com/superfly/flyctl/issues/new?assignees=&labels=bug&template=flyctl-bug-report.md&title=", ip.Address, ip.Type)
		}
	}
	if len(v4s) == 0 && len(v6s) == 0 {
		lprint(nil, "No ipv4 or ipv6 ip addresses allocated to app %s", appName)
		return nil
	}

	appHostname := app.Hostname
	appFqdn := dns.Fqdn(appHostname)
	dnsClient := &dns.Client{}
	ns, err := getFirstFlyDevNameserver(dnsClient)
	if err != nil {
		lprint(nil, "%s. Can't proceed to check A or AAAA records.\n", err.Error())
		return nil
	}
	nsAddr := fmt.Sprintf("%s:53", strings.TrimSuffix(ns, "."))

	if len(v4s) > 0 {
		lprint(nil, "Checking A record for %s... ", appHostname)
		err, jsonErr := checkDnsRecords(dnsClient, nsAddr, appName, appFqdn, "A", v4s)
		if err == nil {
			lprint(color.Green, "PASSED\n")
			checks["appARecord"] = "ok"
		} else {
			lprint(nil, "%s\n\n", err.Error())
			if jsonErr != "" {
				checks["appARecord"] = jsonErr
			} else {
				checks["appARecord"] = err.Error()
			}
		}
	}

	if len(v6s) > 0 {
		lprint(nil, "Checking AAAA record for %s... ", appHostname)
		err, jsonErr := checkDnsRecords(dnsClient, nsAddr, appName, appFqdn, "AAAA", v6s)
		if err == nil {
			lprint(color.Green, "PASSED\n")
			checks["appAAAARecord"] = "ok"
		} else {
			lprint(nil, "%s\n\n", err.Error())
			if jsonErr != "" {
				checks["appAAAARecord"] = jsonErr
			} else {
				checks["appAAAARecord"] = err.Error()
			}
		}
	}

	// ------------------------------------------------------------

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

	org, err := client.GetOrganizationBySlug(ctx, "personal")
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

func getFirstFlyDevNameserver(dnsClient *dns.Client) (string, error) {
	const resolver = "9.9.9.9:53"
	msg := &dns.Msg{}
	flydev := "fly.dev"
	msg.SetQuestion(dns.Fqdn(flydev), dns.TypeNS)
	msg.RecursionDesired = true
	// TODO: use ipv6 when system supports it
	r, _, err := dnsClient.Exchange(msg, resolver)
	if err != nil {
		return "", err
	}
	if r.Rcode != dns.RcodeSuccess {
		return "", fmt.Errorf("failed to resolve NS record for %s. Got error code: %s", flydev, dns.RcodeToString[r.Rcode])
	}
	for _, a := range r.Answer {
		if ns, ok := a.(*dns.NS); ok {
			return ns.Ns, nil
		}
	}
	return "", fmt.Errorf("no NS records found for %s", flydev)
}

func checkDnsRecords(dnsClient *dns.Client, nsAddr string, appName string, appFqdn string, qType string, appIps map[string]bool) (error, string) {
	msg := &dns.Msg{}
	msg.SetQuestion(appFqdn, dns.StringToType[qType])
	msg.RecursionDesired = true

	r, _, err := dnsClient.Exchange(msg, nsAddr)
	if err != nil {
		return fmt.Errorf("failed to lookup A record for %s: %w", appFqdn, err), ""
	}
	if r.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("invalid result when looking up A record for %s: %s", appFqdn, dns.RcodeToString[r.Rcode]), ""
	}
	dnsIps := make(map[string]bool)
	for _, a := range r.Answer {
		if qType == "A" {
			if aRec, ok := a.(*dns.A); ok {
				dnsIps[aRec.A.String()] = true
			}
		} else if qType == "AAAA" {
			if aRec, ok := a.(*dns.AAAA); ok {
				dnsIps[aRec.AAAA.String()] = true
			}
		}
	}

	ipsOnAppNotInDns := make([]string, 0)
	for appIp := range appIps {
		if _, present := dnsIps[appIp]; !present {
			ipsOnAppNotInDns = append(ipsOnAppNotInDns, appIp)
		}
	}
	ipsInDnsNotInApp := make([]string, 0)
	for dnsIp := range dnsIps {
		if _, present := appIps[dnsIp]; !present {
			ipsInDnsNotInApp = append(ipsInDnsNotInApp, dnsIp)
		}
	}

	if len(ipsOnAppNotInDns) == 0 && len(ipsInDnsNotInApp) == 0 {
		return nil, ""
	} else if len(ipsOnAppNotInDns) > 0 {
		missingIps := strings.Join(ipsOnAppNotInDns, ", ")
		return fmt.Errorf(`Nope
	These IPs are missing from the %s %s record: %s
	This likely means we had an operational issue when we tried to create the record.
	Post in https://community.fly.io/ or send us an email if you have a support plan, and we'll get this fixed`,
			appFqdn, qType, missingIps), fmt.Sprintf("missing these ips from the %s record: %s", qType, missingIps)
	} else { // len(ipsInDnsNotInApp) > 0
		missingIps := strings.Join(ipsInDnsNotInApp, ", ")
		return fmt.Errorf(`Nope
	These IPs are set in the %s record for %s, but they are not associated with the %s app: %s
	This likely means we had an operational issue when we tried to create the record.
	Post in https://community.fly.io/ or send us an email if you have a support plan, and we'll get this fixed`,
			qType, appFqdn, appName, missingIps), fmt.Sprintf("extra ips on %s record not associated with app: %s", qType, missingIps)
	}
}
