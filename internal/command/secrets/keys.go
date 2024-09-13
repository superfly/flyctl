package secrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
)

type SecretType = string

const (
	SECRET_TYPE_KMS_HS256          = fly.SECRET_TYPE_KMS_HS256
	SECRET_TYPE_KMS_HS384          = fly.SECRET_TYPE_KMS_HS384
	SECRET_TYPE_KMS_HS512          = fly.SECRET_TYPE_KMS_HS512
	SECRET_TYPE_KMS_XAES256GCM     = fly.SECRET_TYPE_KMS_XAES256GCM
	SECRET_TYPE_KMS_NACL_AUTH      = fly.SECRET_TYPE_KMS_NACL_AUTH
	SECRET_TYPE_KMS_NACL_BOX       = fly.SECRET_TYPE_KMS_NACL_BOX
	SECRET_TYPE_KMS_NACL_SECRETBOX = fly.SECRET_TYPE_KMS_NACL_SECRETBOX
	SECRET_TYPE_KMS_NACL_SIGN      = fly.SECRET_TYPE_KMS_NACL_SIGN
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

// getFlapsClient builds and returns a flaps client for the App from the context.
func getFlapsClient(ctx context.Context) (*flaps.Client, error) {
	client := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create flaps client: %w", err)
	}
	return flapsClient, nil
}
