package secrets

import (
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
)

type SecretType = string

const (
	SECRETKEY_TYPE_HS256          = fly.SECRETKEY_TYPE_HS256
	SECRETKEY_TYPE_HS384          = fly.SECRETKEY_TYPE_HS384
	SECRETKEY_TYPE_HS512          = fly.SECRETKEY_TYPE_HS512
	SECRETKEY_TYPE_XAES256GCM     = fly.SECRETKEY_TYPE_XAES256GCM
	SECRETKEY_TYPE_NACL_AUTH      = fly.SECRETKEY_TYPE_NACL_AUTH
	SECRETKEY_TYPE_NACL_BOX       = fly.SECRETKEY_TYPE_NACL_BOX
	SECRETKEY_TYPE_NACL_SECRETBOX = fly.SECRETKEY_TYPE_NACL_SECRETBOX
	SECRETKEY_TYPE_NACL_SIGN      = fly.SECRETKEY_TYPE_NACL_SIGN
	SECRETKEY_TYPE_ES256          = fly.SECRETKEY_TYPE_ES256
)

func newKeys() *cobra.Command {
	const (
		long = `Keys are available to applications through the /.fly/kms filesystem. Names are case
		sensitive and stored as-is, so ensure names are appropriate as filesystem names.
		Names optionally include version information with a "vN" suffix.
		`

		short = "Manage application key secrets with the gen, list, and delete commands."
	)

	keys := command.New("keys", short, long, nil)

	keys.AddCommand(
		newKeysList(),
		newKeyGenerate(),
		newKeyDelete(),
	)

	keys.Hidden = true // TODO: unhide when we're ready to go public.

	return keys
}

// secretTypeToString converts from a standard sType to flyctl's abbreviated string form.
func secretTypeToString(sType string) string {
	return strings.TrimPrefix(strings.ToLower(sType), "secret_type_kms_")
}
