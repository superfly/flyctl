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
		long = `Applications use keys through the Machines API, from inside a Machine, over the
/.fly/api socket (root only). The /.fly/kms filesystem is not mounted on Machines
by default, so don't look for keys there. The decrypt, sign and verify routes sit
next to the encrypt route shown in the example. From outside a Machine,
https://api.machines.dev serves the same paths, but refuses encrypt, decrypt and
sign. Names are case sensitive and stored as-is. Names optionally include version
information with a "vN" suffix; the API needs the full label, including the version.
`

		example = `  # Inside a Machine: encrypt with an encrypting key (plaintext is base64)
  curl --unix-socket /.fly/api -X POST \
    -H 'Content-Type: application/json' \
    -d '{"plaintext":"aGVsbG8="}' \
    http://flaps/v1/apps/<app>/secretkeys/<label>/encrypt`

		short = "Manage application key secrets with the gen, list, and delete commands."
	)

	keys := command.New("keys", short, long, nil)
	keys.Example = example

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
