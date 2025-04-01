package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newKeyDelete() (cmd *cobra.Command) {
	const (
		long  = `Delete the application key secret by label.`
		short = `Delete the application key secret`
		usage = "delete [flags] label"
	)

	cmd = command.New(usage, short, long, runKeyDelete, command.RequireSession, command.RequireAppName)

	cmd.Aliases = []string{"rm"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "Force deletion without prompting",
		},
		flag.Bool{
			Name:        "noversion",
			Shorthand:   "n",
			Default:     false,
			Description: "do not automatically match all versions of a key when version is unspecified. all matches must be explicit",
		},
	)

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runKeyDelete(ctx context.Context) (err error) {
	label := flag.Args(ctx)[0]
	ver, prefix, err := SplitLabelKeyver(label)
	if err != nil {
		return err
	}

	flapsClient, err := getFlapsClient(ctx)
	if err != nil {
		return err
	}

	secrets, err := flapsClient.ListSecrets(ctx)
	if err != nil {
		return err
	}

	// Delete all matching secrets, prompting if necessary.
	var rerr error
	out := iostreams.FromContext(ctx).Out
	for _, secret := range secrets {
		ver2, prefix2, err := SplitLabelKeyver(secret.Label)
		if err != nil {
			continue
		}
		if prefix != prefix2 {
			continue
		}

		if ver != ver2 {
			// Subtle: If the `noversion` flag was specified, then we must have
			// an exact match. Otherwise if version is unspecified, we
			// match all secrets with the same version regardless of version.
			if flag.GetBool(ctx, "noversion") {
				continue
			}
			if ver != KeyverUnspec {
				continue
			}
		}

		if !flag.GetBool(ctx, "force") {
			confirm, err := prompt.Confirm(ctx, fmt.Sprintf("delete secrets key %s?", secret.Label))
			if err != nil {
				rerr = errors.Join(rerr, err)
				continue
			}
			if !confirm {
				continue
			}
		}

		err = flapsClient.DeleteSecret(ctx, secret.Label)
		if err != nil {
			var ferr *flaps.FlapsError
			if errors.As(err, &ferr) && ferr.ResponseStatusCode == 404 {
				err = fmt.Errorf("not found")
			}
			rerr = errors.Join(rerr, fmt.Errorf("deleting %v: %w", secret.Label, err))
		} else {
			fmt.Fprintf(out, "Deleted %v\n", secret.Label)
		}
	}
	return rerr
}
