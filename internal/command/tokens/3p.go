package tokens

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/terminal"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
)

func new3P() *cobra.Command {
	const (
		short = "Manage third-party (3P) caveats for Fly.io API tokens"
		long  = `Add, manage, and discharge third-party tokens for a Fly.io API token. 
The token to be manipulated may either be passed in the -t argument or in FLY_API_TOKEN.
Third-party caveats rely on a secret shared with between the third party and the 
author of the caveat. Pass this secret with --secret, --secret-file, or through the
TOKEN_3P_SHARED_SECRET variable.
`
		usage = "3p"
	)

	cmd := command.New(usage, short, long, nil)
	cmd.Aliases = []string{"third-party"}

	cmd.AddCommand(
		new3PAdd(),
		new3PTicket(),
		new3PDischarge(),
		new3PAddDischarge(),
	)

	return cmd
}

func getRootToken(ctx context.Context) (*macaroon.Macaroon, error) {
	toks, err := getTokens(ctx)
	if err != nil {
		return nil, err
	}

	// a "token" is actually a little bag full of tokens; we only care about the
	// root "permissions" token, the one our API issued, and not any of the discharge
	// tokens accompanying it

	permMacs, _, _, _, err := macaroon.FindPermissionAndDischargeTokens(toks, flyio.LocationPermission)
	if err != nil {
		return nil, err
	}

	switch len(permMacs) {
	case 0:
		return nil, errors.New("not a fly.io token")
	case 1:
		return permMacs[0], nil
	default:
		return nil, errors.New("multiple fly.io permission tokens")
	}
}

func get3PSharedSecret(ctx context.Context) ([]byte, error) {
	var (
		s64 string
	)

	path := flag.GetString(ctx, "secret-file")
	if path != "" {
		b64, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read shared secret from %s: %w", path, err)
		}

		s64 = string(b64)
	} else {
		s64 = os.Getenv("TOKEN_3P_SHARED_SECRET")
		if s64 == "" {
			s64 = flag.GetString(ctx, "secret")
			if s64 == "" {
				return nil, errors.New("provide third-party shared secret with --secret-file, TOKEN_3P_SHARED_SECRET, or --secret")
			}

			terminal.Warnf("passing secrets with --secret is risky, prefer --secret-file or TOKEN_3P_SHARED_SECRET")
		}
	}

	secret, err := base64.StdEncoding.DecodeString(s64)
	if err == nil && len(secret) != 32 {
		err = errors.New("shared secret should be 32 bytes long (once decoded)")
	}
	if err != nil {
		return nil, fmt.Errorf("decode third-party shared secret: %w", err)
	}

	return secret, nil
}

func new3PAdd() *cobra.Command {
	const (
		short = "Add a third-party caveat"
		long  = `Add a caveat to the Fly.io token that requires a third-party service, 
identified by --location (a URL), to supply a discharge token in order to clear.
`
		usage = "add"
	)

	cmd := command.New(usage, short, long, run3PAdd)

	flag.Add(cmd,
		flag.String{
			Name:        "secret",
			Shorthand:   "S",
			Description: "(insecure) base64 shared secret for third-party caveat",
		},
		flag.String{
			Name:        "secret-file",
			Shorthand:   "s",
			Description: "file containing base64 shared secret for third-party caveat",
		},
		flag.String{
			Name:        "location",
			Shorthand:   "l",
			Description: "URL identifying third-party service",
		},
	)

	return cmd
}

func new3PTicket() *cobra.Command {
	const (
		short = "Retrieve the ticket from an existing third-party caveat"
		long  = `If a third-party caveat tied to the URL at --location is present in
the Fly.io API token, retrieve its ticket, so it can be submitted to the service
to retrieve a discharge token.`
		usage = "ticket"
	)

	cmd := command.New(usage, short, long, run3PTicket)

	flag.Add(cmd,
		flag.String{
			Name:        "location",
			Shorthand:   "l",
			Description: "URL identifying third-party service",
		},
	)

	return cmd
}

func new3PDischarge() *cobra.Command {
	const (
		short = "Exchange a ticket for the token that discharges a third-party caveat"
		long  = `Given the ticket for a third-party caveat, generate the discharge token
that satisfies the caveat.`
		usage = "discharge"
	)

	cmd := command.New(usage, short, long, run3PDischarge)

	flag.Add(cmd,
		flag.String{
			Name:        "secret",
			Shorthand:   "S",
			Description: "(insecure) base64 shared secret for third-party caveat",
		},
		flag.String{
			Name:        "secret-file",
			Shorthand:   "s",
			Description: "file containing base64 shared secret for third-party caveat",
		},
		flag.String{
			Name:        "ticket",
			Description: "Third party caveat ticket",
		},
		flag.String{
			Name:        "location",
			Shorthand:   "l",
			Description: "URL identifying third-party service",
		},
	)

	return cmd
}

func new3PAddDischarge() *cobra.Command {
	const (
		short = "Tack a discharge token onto a Fly.io API token"
		long  = `Once obtained by exchanging a caveat ticket with a third-party service,
add the matching discharge token to the Fly.io API token header, to include it with
authentication attempts.`
		usage = "add-discharge"
	)

	cmd := command.New(usage, short, long, run3PAddDischarge)

	flag.Add(cmd,
		flag.String{
			Name:        "discharge",
			Shorthand:   "d",
			Description: "Third-party discharge token",
		},
	)

	return cmd
}

func run3PAdd(ctx context.Context) error {
	loc := flag.GetString(ctx, "location")
	if loc == "" {
		return errors.New("provide a --location URL for the third-party caveat")
	}

	secret, err := get3PSharedSecret(ctx)
	if err != nil {
		return err
	}

	m, err := getRootToken(ctx)
	if err != nil {
		return err
	}

	m.Add3P(secret, loc)

	tok, err := m.Encode()
	if err != nil {
		return err
	}

	// get the discharge tokens
	toks, err := getTokens(ctx)
	if err != nil {
		return err
	}

	_, _, _, disToks, err := macaroon.FindPermissionAndDischargeTokens(toks, flyio.LocationPermission)
	if err != nil {
		return err
	}

	toks = append(disToks, tok)

	fmt.Println(macaroon.ToAuthorizationHeader(toks...))

	return nil
}

func run3PTicket(ctx context.Context) error {
	loc := flag.GetString(ctx, "location")
	if loc == "" {
		return errors.New("provide a --location URL for the third-party caveat")
	}

	m, err := getRootToken(ctx)
	if err != nil {
		return err
	}

	ticket, err := m.ThirdPartyCID(loc, nil)
	if err != nil {
		return err
	}

	if len(ticket) == 0 {
		return errors.New("no 3p ticket in token")
	}

	fmt.Println(base64.StdEncoding.EncodeToString(ticket))

	return nil
}

func run3PDischarge(ctx context.Context) error {
	loc := flag.GetString(ctx, "location")
	if loc == "" {
		return errors.New("provide a --location URL for the third-party caveat")
	}

	secret, err := get3PSharedSecret(ctx)
	if err != nil {
		return err
	}

	ticket64 := flag.GetString(ctx, "ticket")
	if ticket64 == "" {
		return errors.New("provide a --ticket to discharge")
	}

	ticket, err := base64.StdEncoding.DecodeString(ticket64)
	if err != nil {
		return err
	}

	cavs, dm, err := macaroon.DischargeCID(secret, loc, ticket)
	if err != nil {
		return err
	}
	if len(cavs) != 0 {
		return errors.New("ticket contains caveats that must be checked; use API to discharge")
	}

	tok, err := dm.Encode()
	if err != nil {
		return err
	}

	fmt.Println(base64.StdEncoding.EncodeToString(tok))

	return nil
}

func run3PAddDischarge(ctx context.Context) error {
	toks, err := getTokens(ctx)
	if err != nil {
		return err
	}

	dis64 := flag.GetString(ctx, "discharge")
	if dis64 == "" {
		return errors.New("provide a discharge token with --discharge")
	}

	tok, err := base64.StdEncoding.DecodeString(dis64)
	if err != nil {
		return err
	}

	toks = append(toks, tok)

	fmt.Println(macaroon.ToAuthorizationHeader(toks...))

	return nil
}
