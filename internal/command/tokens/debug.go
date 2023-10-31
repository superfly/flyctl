package tokens

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

func newDebug() *cobra.Command {
	const (
		short = "Debug Fly.io API tokens"
		long  = `Decode and print a Fly.io API token. The token to be
				debugged may either be passed in the -t argument or in FLY_API_TOKEN.
				See https://github.com/superfly/macaroon for details Fly.io macaroon
				tokens.`
		usage = "debug"
	)

	cmd := command.New(usage, short, long, runDebug)

	flag.Add(cmd,
		flag.String{
			Name:        "file",
			Shorthand:   "f",
			Description: "Filename to read caveats from. Defaults to stdin",
		},
	)

	return cmd
}

type mappings struct {
	orgs, apps map[int64]string
}

func retrieveMappings(ctx context.Context) (ret mappings, err error) {
	ret.orgs = map[int64]string{}
	ret.apps = map[int64]string{}

	client := client.FromContext(ctx)
	apps, err := client.API().GetApps(ctx, nil)
	if err != nil {
		return ret, err
	}

	for _, app := range apps {
		oid, _ /* never happening */ := strconv.ParseInt(app.Organization.InternalNumericID, 10, 64)

		ret.apps[app.InternalNumericID] = app.Name
		ret.orgs[oid] = app.Organization.Slug
	}

	return
}

func runDebug(ctx context.Context) error {
	toks, err := getTokens(ctx)
	if err != nil {
		return err
	}

	macs := make([]*macaroon.Macaroon, 0, len(toks))

	for i, tok := range toks {
		m, err := macaroon.Decode(tok)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to decode token at position %d: %s\n", i, err)
			continue
		}
		macs = append(macs, m)
	}

	maps, err := retrieveMappings(ctx)
	if err != nil {
		return err
	}

	for _, mac := range macs {
		printMacaroon(ctx, maps, mac)
		println("")
	}

	if !flag.GetBool(ctx, "verbose") {
		return nil
	}

	// encode to buffer to avoid failing halfway through
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(macs); err != nil {
		return fmt.Errorf("unable to encode tokens: %w", err)
	}
	fmt.Println(buf.String())

	return nil
}

func printActions(ctx context.Context, act resset.Action) string {
	bits := []resset.Action{resset.ActionRead, resset.ActionWrite, resset.ActionCreate, resset.ActionDelete, resset.ActionControl}
	bitmap := map[resset.Action]string{
		resset.ActionRead:    "read",
		resset.ActionWrite:   "write",
		resset.ActionCreate:  "create",
		resset.ActionDelete:  "delete",
		resset.ActionControl: "control",
	}

	var everything resset.Action
	for _, b := range bits {
		everything |= b
	}

	if act&everything == everything {
		return "everything"
	}

	if act == resset.ActionRead {
		return "read-only"
	}

	actions := []string{}

	for _, b := range bits {
		if act&b == b {
			actions = append(actions, bitmap[b])
		}
	}

	if len(actions) == 1 {
		return actions[0]
	}

	return strings.Join(actions, ", ")
}

func timeHighOrder(d time.Duration) string {
	switch {
	case d > time.Hour*24:
		return fmt.Sprintf("%d days", d.Round(time.Hour*24)/(time.Hour*24))
	case d > time.Hour:
		return fmt.Sprintf("%d hours", d.Round(time.Hour)/time.Hour)
	default:
		return fmt.Sprintf("%d minutes", d.Round(time.Minute)/time.Minute)
	}
}

func printMacaroon(ctx context.Context, maps mappings, m *macaroon.Macaroon) error {
	caveats := m.UnsafeCaveats.Caveats

	lookup := func(x uint64, mp map[int64]string) string {
		if v, ok := mp[int64(x)]; ok {
			return v
		}
		return fmt.Sprintf("%d", x)
	}

	kid := m.Nonce.KID
	if len(kid) > 6 {
		kid = kid[len(kid)-6:]
	}

	fmt.Printf("Token ...%x (from %s)\n", kid, m.Location)
	if m.Location == flyio.LocationAuthentication {
		fmt.Printf("This is an authentication token for Fly.io\n")
	} else if m.Location == flyio.LocationPermission {
		fmt.Printf("This is a root permission token for Fly.io\n")
	}
	fmt.Printf("Caveats in this token:\n")

	depth := 0

	dep := func(f func()) {
		depth += 1
		f()
		depth -= 1
	}

	dprint := func(format string, args ...interface{}) {
		tabs := ""

		for i := 0; i < depth; i++ {
			tabs += "\t"
		}

		fmt.Printf(tabs+format+"\n", args...)
	}

	stringset := func(ld, kinds, kind string, rs resset.ResourceSet[string]) {
		dprint("%s for the following %s:", ld, kinds)
		for ft, axs := range rs {
			dep(func() {
				dprint("For %s '%s', allowed actions: %s", kind, ft, printActions(ctx, axs))
			})
		}
	}

	for i, ocav := range caveats {
		leadin := "* And exclusively"
		if i == 0 {
			leadin = "* Exclusively"
		}

		dep(func() {
			switch cav := ocav.(type) {
			case *flyio.Organization:
				dprint("%s for organization '%s'", leadin, lookup(cav.ID, maps.orgs))
				dep(func() {
					dprint("Allowed actions: %s", printActions(ctx, cav.Mask))
				})
			case *flyio.Apps:
				dprint("%s for the following apps:", leadin)
				for appid, axs := range cav.Apps {
					dep(func() {
						dprint("For app '%s', allowed actions: %s", lookup(appid, maps.apps), printActions(ctx, axs))
					})
				}
			case *flyio.FeatureSet:
				stringset(leadin, "features", "feature", cav.Features)
			case *flyio.Volumes:
				stringset(leadin, "volumes", "volume", cav.Volumes)
			case *flyio.Machines:
				stringset(leadin, "machines", "machine", cav.Machines)
			case *flyio.MachineFeatureSet:
				stringset(leadin, "machine features", "machine feature", cav.Features)
			case *flyio.Mutations:
				dprint("%s for the following GraphQL API mutations: %s", leadin, strings.Join(cav.Mutations, ", "))
			case *flyio.Clusters:
				stringset(leadin, "clusters", "cluster", cav.Clusters)
			case *macaroon.ValidityWindow:
				var (
					now       = time.Now()
					before    = time.Unix(cav.NotBefore, 0)
					after     = time.Unix(cav.NotAfter, 0)
					beforeAgo = now.Sub(before)
					afterAgo  = after.Sub(now)
					valid     = false
				)

				if now.After(before) && now.Before(after) {
					valid = true
				}

				validity := "and is currently valid"
				if !valid {
					validity = "and is INVALID"
				}

				dprint("* This token expires (%s)", validity)
				dep(func() {
					dprint("Valid as of: %s ago", timeHighOrder(beforeAgo))
					dprint("Valid until: %s from now", timeHighOrder(afterAgo))
				})

			case *macaroon.Caveat3P:
				switch cav.Location {
				case flyio.LocationAuthentication:
					dprint("* Requires authentication to Fly.io")
				default:
					dprint("* This token can only be satisfied by talking to %s", cav.Location)
				}
			default:
				dprint("* Internal caveat (you probably don't care): %T %+v", cav, cav)
			}
		})
	}

	return nil
}
