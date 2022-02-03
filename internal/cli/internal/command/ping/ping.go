package ping

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv6"
)

func New() *cobra.Command {
	var (
		long = strings.Trim(`
Test connectivity with ICMP ping messages.

This runs over WireGuard; tell us which WireGuard tunnel to use by 
running from within an app directory (with a 'fly.toml'), passing the
'-a' flag with an app name, or the '-o' flag with an org name.

With no arguments, test connectivity to your gateway, the first hop
in our network, to see if your WireGuard connection is working.

The target argument can be either a ".internal" DNS name in our network
(the name of your application) or "gateway".
`, "\n")
		short = `Test connectivity with ICMP ping messages`
	)

	cmd := command.New("ping [hostname] [flags]", short, long, run,
		command.RequireSession, command.LoadAppNameIfPresent)

	cmd.Args = cobra.RangeArgs(0, 1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.String{
			Name:        "interval",
			Shorthand:   "i",
			Default:     "1s",
			Description: "Interval between ping probes",
		},
		flag.Int{
			Name:        "count",
			Shorthand:   "n",
			Default:     0,
			Description: "Number of probes to send (0=indefinite)",
		},
		flag.Int{
			Name:        "size",
			Shorthand:   "s",
			Default:     12,
			Description: "Size of probe to send (not including headers)",
		},
	)

	return cmd
}

func FindApps(ctx context.Context, r *agent.Resolver) (map[string]string, error) {
	txts, err := r.LookupTXT(ctx, "_apps.internal")
	if err != nil {
		return nil, fmt.Errorf("find apps: %w", err)
	}

	mu := sync.Mutex{}
	targets := map[string]string{}
	errs := map[string]error{}

	wg := sync.WaitGroup{}

	for _, app := range strings.Split(strings.Join(txts, ""), ",") {
		go func(app string) {
			wg.Add(1)
			defer wg.Done()

			hostname := fmt.Sprintf("top1.nearest.of.%s.internal", app)
			addrs, err := r.LookupHost(ctx, hostname)
			if err != nil {
				mu.Lock()
				defer mu.Unlock()
				errs[app] = err
				return
			}

			// BUG(tqbf): off the top of my head I don't know if I need this check
			if len(addrs) == 0 {
				mu.Lock()
				defer mu.Unlock()
				errs[app] = errors.New("no records for app")
				return
			}

			mu.Lock()
			defer mu.Unlock()
			targets[addrs[0]] = app + ".internal"
		}(app)
	}

	wg.Wait()

	if len(targets) == 0 {
		if len(errs) == 0 {
			return nil, nil
		}

		allErrs := []string{}

		for k, v := range errs {
			allErrs = append(allErrs, fmt.Sprintf("resolve %s: %s", k, v))
		}

		return nil, fmt.Errorf("find apps: %s", strings.Join(allErrs, ", "))
	}

	return targets, nil
}

func run(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	var (
		org  *api.Organization
		err  error
		name = flag.FirstArg(ctx)
	)

	switch {
	case name == "":
	case name == "gateway":
	case name == "apps":
	case strings.HasSuffix(name, ".internal"):
	case strings.HasPrefix(name, "fdaa:"):
		if net.ParseIP(name) == nil {
			return fmt.Errorf("bad target name: malformed 6pn address")
		}
	default:
		return fmt.Errorf("bad target name: Fly.io DNS names end in '.internal'")
	}

	// BUG(tqbf): DRY this up with dig

	orgSlug := flag.GetOrg(ctx)

	switch orgSlug {
	case "":
		appName := app.NameFromContext(ctx)

		app, err := client.GetApp(ctx, appName)
		if err != nil {
			return fmt.Errorf("get app: %w", err)
		}
		org = &app.Organization
	default:
		org, err = client.FindOrganizationBySlug(ctx, orgSlug)
		if err != nil {
			if err != nil {
				return fmt.Errorf("look up org: %w", err)
			}
		}
	}

	aClient, err := agent.Establish(ctx, client)
	if err != nil {
		return err
	}

	r, err := aClient.Resolver(ctx, org.Slug)
	if err != nil {
		return err
	}

	ns := r.NSAddr()

	var mu sync.RWMutex
	targets := map[string]string{}

	mu.Lock()
	if name == "" || name == "gateway" {
		targets[ns] = "gateway"
	} else if strings.HasPrefix(name, "fdaa:") {
		targets[name] = name
	} else if name == "apps" {
		fmt.Printf("- hunting down your deployed apps...\n")
		targets, err = FindApps(ctx, r)
		if err != nil {
			return err
		}
	} else {
		addrs, err := r.LookupHost(ctx, name)
		if err != nil {
			return fmt.Errorf("look up %s: %w", name, err)
		}

		for _, a := range addrs {
			targets[a] = name
		}
	}
	mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)

	if name != "" && name != "apps" && name != "gateway" && !strings.HasPrefix(name, "fdaa:") {
		// look up names in the background because I was too
		// lazy to implement PTR in our DNS server
		go func() {
			// we already checked the format of this string
			labels := strings.Split(name, ".internal")
			app := labels[len(labels)-2]
			regionName := fmt.Sprintf("regions.%s.internal", app)

			regionFrags, err := r.LookupTXT(ctx, regionName)
			if err != nil {
				return
			}

			regions := strings.Join(regionFrags, "")

			wg := sync.WaitGroup{}

			for _, region := range strings.Split(regions, ",") {
				go func(region string) {
					wg.Add(1)
					defer wg.Done()

					regHost := fmt.Sprintf("%s.%s.internal", region, app)
					addrs, err := r.LookupHost(ctx, regHost)
					if err == nil {
						mu.Lock()
						for _, addr := range addrs {
							targets[addr] = regHost
						}
						mu.Unlock()
					}

					wg.Wait()
				}(region)
			}
		}()
	}

	pinger, err := aClient.Pinger(ctx, org.Slug)
	if err != nil {
		return err
	}

	ivString := flag.GetString(ctx, "interval")
	interval, err := time.ParseDuration(ivString)
	if err != nil {
		return err
	}

	if interval < (100 * time.Millisecond) {
		interval = 100 * time.Millisecond
	}

	count := flag.GetInt(ctx, "count")

	pad := uint(flag.GetInt(ctx, "size"))
	if pad > 1000 {
		pad = 1000
	}

	ticker := time.NewTicker(interval)

	var timeLen = 0

	msg := func(id, seq int, t time.Time, pad uint) []byte {
		tbuf, _ := t.MarshalBinary()
		timeLen = len(tbuf)
		buf := &bytes.Buffer{}
		buf.Write(tbuf)
		buf.Grow(int(pad))
		for i := uint(0); i < pad; i++ {
			buf.WriteByte('A')
		}

		msg := icmp.Message{
			Type: ipv6.ICMPTypeEchoRequest,
			Code: 0,
			Body: &icmp.Echo{
				ID:   id,
				Seq:  seq,
				Data: buf.Bytes(),
			},
		}

		raw, err := msg.Marshal(nil)
		if err != nil {
			log.Panicf("marshal icmp: %s", err)
		}

		return raw
	}

	type reply struct {
		src net.Addr
		pkt *icmp.Echo
		lat time.Duration
	}

	replies := make(chan reply, 2)

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}

			var (
				replyBuf = make([]byte, 1500)
				rmsg     *icmp.Message
				echoRep  *icmp.Echo
				ok       bool
			)

			pinger.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n64, raddr, err := pinger.ReadFrom(replyBuf)
			if err != nil {
				continue
			}

			rmsg, err = icmp.ParseMessage(58, replyBuf[:n64])
			if err == nil {
				echoRep, ok = rmsg.Body.(*icmp.Echo)
			}
			if err != nil || !ok || len(echoRep.Data) < timeLen {
				fmt.Printf("bogus ICMP from %s: %s", raddr, err)
				continue
			}

			var t time.Time
			err = t.UnmarshalBinary(echoRep.Data[:timeLen])
			if err != nil {
				fmt.Printf("malformed timestamp from %s: %s", raddr, err)
			}

			replies <- reply{
				src: raddr,
				pkt: echoRep,
				lat: time.Now().Sub(t),
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case reply := <-replies:
				mu.RLock()
				srcName := targets[reply.src.String()]
				mu.RUnlock()

				if srcName != "" {
					srcName = " (" + srcName + ")"
				}

				lat := reply.lat.Truncate(100 * time.Microsecond)

				fmt.Printf("%d bytes from %s%s, seq=%d time=%s\n", len(reply.pkt.Data)+8, reply.src, srcName, reply.pkt.Seq, lat)
			}
		}
	}()

	stp := make(chan os.Signal, 1)
	signal.Notify(stp, syscall.SIGINT, syscall.SIGTERM)

	for i := 0; count == 0 || i < count; i++ {
		select {
		case <-stp:
			cancel()
			return nil
		case <-ticker.C:
		}

		for target := range targets {
			// BUG(tqbf): stop re-parsing these stupid addresses
			_, err = pinger.WriteTo(msg(0, i, time.Now(), pad), &net.IPAddr{IP: net.ParseIP(target)})
			if err != nil {
				return err
			}
		}
	}

	cancel()

	return nil
}
