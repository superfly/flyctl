package tokens

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/macaroon"
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
