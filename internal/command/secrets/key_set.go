package secrets

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newKeySet() (cmd *cobra.Command) {
	const (
		long = `Set the application key secret. If the label is not fully qualified
with a version, and a secret with the same label already exists, the label will be
updated to include the next version number. If a base64 value is not provided, the
key will be generated with a randomly generated value.`
		short = `Set the application key secret`
		usage = "set [flags] type label [base64value]"
	)

	cmd = command.New(usage, short, long, runKeySet, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "noversion",
			Shorthand:   "n",
			Default:     false,
			Description: "do not automatically version the key label",
		},
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Don't print key label",
		},
	)

	cmd.Args = cobra.RangeArgs(2, 3)

	return cmd
}

func runKeySet(ctx context.Context) (err error) {
	out := iostreams.FromContext(ctx).Out
	args := flag.Args(ctx)
	typStr := args[0]
	label := args[1]
	val := []byte{}

	typ, err := secretTypeFromString(typStr)
	if err != nil {
		return err
	}

	if len(args) > 2 {
		val, err = base64.StdEncoding.DecodeString(args[2])
		if err != nil {
			return fmt.Errorf("bad value encoding: %w", err)
		}
	}

	flapsClient, err := getFlapsClient(ctx)
	if err != nil {
		return err
	}

	secrets, err := flapsClient.ListSecrets(ctx)
	if err != nil {
		return err
	}

	// Verify consistency with existing keys with the same prefix
	// while finding the highest version with the same prefix.
	ver, prefix := splitLabelVersion(label)
	bestVer := VerUnspec
	for _, secret := range secrets {
		ver2, prefix2 := splitLabelVersion(secret.Label)
		if prefix != prefix2 {
			continue
		}

		if secret.Type != typ {
			typs := secretTypeToString(secret.Type)
			return fmt.Errorf("key %v (%v) has conflicting type %v", prefix, secret.Label, typs)
		}

		if CompareVer(ver2, bestVer) > 0 {
			bestVer = ver2
		}
	}

	// If the label does not contain an explicit version,
	// we will automatically apply a version to the label
	// unless the user said not to.
	if ver == VerUnspec && !flag.GetBool(ctx, "noversion") {
		ver, err := bestVer.Incr()
		if err != nil {
			return err
		}
		label = joinLabelVersion(ver, prefix)
	}

	if !flag.GetBool(ctx, "quiet") {
		typs := secretTypeToString(typ)
		fmt.Fprintf(out, "Setting %s (%s)\n", label, typs)
	}

	err = flapsClient.CreateSecret(ctx, fly.CreateSecretRequest{
		Label: label,
		Type:  typ,
		Value: val,
	})
	if err != nil {
		return err
	}
	return nil
}
