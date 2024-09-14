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

func newKeyGenerate() (cmd *cobra.Command) {
	const (
		long = `Generate a random application key secret. If the label is not fully qualified
with a version, and a secret with the same label already exists, the label will be
updated to include the next version number.`
		short = `Generate the application key secret`
		usage = "generate [flags] type label"
	)

	cmd = command.New(usage, short, long, runKeySetOrGenerate, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "Force overwriting existing values",
		},
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

	cmd.Aliases = []string{"gen"}
	cmd.Args = cobra.ExactArgs(2)

	return cmd
}

// runKeySetOrGenerate handles both `keys set typ label value` and
// `keys generate typ label`. The sole difference is whether a `value`
// arg is present or not.
func runKeySetOrGenerate(ctx context.Context) (err error) {
	out := iostreams.FromContext(ctx).Out
	args := flag.Args(ctx)
	semType := SemanticType(args[0])
	label := args[1]
	val := []byte{}

	ver, prefix, err := SplitLabelKeyver(label)
	if err != nil {
		return err
	}

	typ, err := SemanticTypeToSecretType(semType)
	if err != nil {
		return err
	}

	gen := true
	if len(args) > 2 {
		gen = false
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
	bestVer := KeyverUnspec
	for _, secret := range secrets {
		if label == secret.Label {
			if !flag.GetBool(ctx, "force") {
				return fmt.Errorf("refusing to overwrite existing key")
			}
		}

		ver2, prefix2, err := SplitLabelKeyver(secret.Label)
		if err != nil {
			continue
		}
		if prefix != prefix2 {
			continue
		}

		// The semantic type must be the same as any existing keys with the same label prefix.
		semType2, _ := SecretTypeToSemanticType(secret.Type)
		if semType2 != semType {
			typs := secretTypeToString(secret.Type)
			return fmt.Errorf("key %v (%v) has conflicting type %v (%v)", prefix, secret.Label, semType2, typs)
		}

		if CompareKeyver(ver2, bestVer) > 0 {
			bestVer = ver2
		}
	}

	// If the label does not contain an explicit version,
	// we will automatically apply a version to the label
	// unless the user said not to.
	if ver == KeyverUnspec && !flag.GetBool(ctx, "noversion") {
		ver, err := bestVer.Incr()
		if err != nil {
			return err
		}
		label = JoinLabelVersion(ver, prefix)
	}

	if !flag.GetBool(ctx, "quiet") {
		typs := secretTypeToString(typ)
		fmt.Fprintf(out, "Setting %s %s (%s)\n", label, semType, typs)
	}

	if gen {
		err = flapsClient.GenerateSecret(ctx, label, typ)
	} else {
		err = flapsClient.CreateSecret(ctx, label, typ, fly.CreateSecretRequest{Value: val})
	}
	if err != nil {
		return err
	}
	return nil
}
