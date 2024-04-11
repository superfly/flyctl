package tokens

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
)

func newAttenuate() *cobra.Command {
	const (
		short = "Attenuate Fly.io API tokens"
		long  = `Attenuate a Fly.io API token by appending caveats to it. The
				token to be attenuated may either be passed in the -t argument
				or in FLY_API_TOKEN. Caveats must be JSON encoded. See
				https://github.com/superfly/macaroon for details on
				macaroons and caveats.`
		usage = "attenuate"
	)

	cmd := command.New(usage, short, long, runAttenuate)

	flag.Add(cmd,
		flag.String{
			Name:        "file",
			Shorthand:   "f",
			Description: "Filename to read caveats from. Defaults to stdin",
		},
		flag.String{
			Name:        "location",
			Shorthand:   "l",
			Description: "Location identifier of macaroons to attenuate",
			Default:     flyio.LocationPermission,
			Hidden:      true,
		},
	)

	return cmd
}

func runAttenuate(ctx context.Context) error {
	macs, _, _, disToks, err := getPermissionAndDischargeTokens(ctx)
	if err != nil {
		return err
	}

	cavs, err := getCaveats(ctx)
	if err != nil {
		return err
	}

	for _, m := range macs {
		if err := m.Add(cavs.Caveats...); err != nil {
			return fmt.Errorf("unable to attenuate macaroon: %w", err)
		}
	}

	return encodeAndPrintToken(macs, nil, nil, disToks)
}

func getPermissionAndDischargeTokens(ctx context.Context) ([]*macaroon.Macaroon, [][]byte, []*macaroon.Macaroon, [][]byte, error) {
	toks, err := getTokens(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	location := flag.GetString(ctx, "location")
	macs, macToks, diss, disToks, err := macaroon.FindPermissionAndDischargeTokens(toks, location)
	switch {
	case err != nil:
		return nil, nil, nil, nil, fmt.Errorf("unable to decode token: %w", err)
	case len(macs) == 0:
		return nil, nil, nil, nil, fmt.Errorf("no %s permission tokens found", location)
	}

	return macs, macToks, diss, disToks, nil
}

func getTokens(ctx context.Context) ([][]byte, error) {
	token := config.Tokens(ctx).MacaroonsOnly().All()

	if token == "" {
		return nil, errors.New("pass a macaroon token (e.g. from `fly tokens deploy`) as the -t argument or in FLY_API_TOKEN")
	}

	toks, err := macaroon.Parse(token)
	switch {
	case errors.Is(err, macaroon.ErrUnrecognizedToken):
		return nil, fmt.Errorf("unable to parse token: %w", err)
	}

	return toks, nil
}

func getCaveats(ctx context.Context) (*macaroon.CaveatSet, error) {
	f := os.Stdin
	if path := flag.GetString(ctx, "file"); path != "" {
		var err error
		if f, err = os.Open(path); err != nil {
			return nil, fmt.Errorf("unable to open file `%s`: %w", path, err)
		}
	}

	dec := json.NewDecoder(f)
	cavs := macaroon.NewCaveatSet()

	if err := dec.Decode(cavs); err != nil {
		return nil, fmt.Errorf("unable to decode caveats: %w", err)
	}

	return cavs, nil
}

// don't pass duplicates in macs/macToks and diss/disToks
func encodeAndPrintToken(macs []*macaroon.Macaroon, macToks [][]byte, diss []*macaroon.Macaroon, disToks [][]byte) error {
	for _, m := range macs {
		mt, err := m.Encode()
		if err != nil {
			return fmt.Errorf("unable to encode macaroon: %w", err)
		}
		macToks = append(macToks, mt)
	}
	for _, d := range diss {
		dt, err := d.Encode()
		if err != nil {
			return fmt.Errorf("unable to encode macaroon: %w", err)
		}
		disToks = append(disToks, dt)
	}

	fmt.Println(macaroon.ToAuthorizationHeader(append(macToks, disToks...)...))
	return nil
}
