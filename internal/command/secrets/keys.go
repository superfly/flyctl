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

var validSecretTypes = []string{
	fly.SECRET_TYPE_KMS_HS256,
	fly.SECRET_TYPE_KMS_HS384,
	fly.SECRET_TYPE_KMS_HS512,
	fly.SECRET_TYPE_KMS_XAES256GCM,
	fly.SECRET_TYPE_KMS_NACL_AUTH,
	fly.SECRET_TYPE_KMS_NACL_BOX,
	fly.SECRET_TYPE_KMS_NACL_SECRETBOX,
	fly.SECRET_TYPE_KMS_NACL_SIGN,
}

func newKeys() *cobra.Command {
	const (
		long = `Keys are available to applications through the /.fly/kms filesystem. Names are case
		sensitive and stored as-is, so ensure names are appropriate as filesystem names.
		Names optionally include version information with a "vN" suffix.
		`

		short = "Manage application key secrets with the set and unset commands."
	)

	keys := command.New("keys", short, long, nil)

	keys.AddCommand(
		newKeysList(),
		newKeySet(),
		newKeyDelete(),
	)

	return keys
}

// secretTypeToString converts from a standard sType to flyctl's abbreviated string form.
func secretTypeToString(sType string) string {
	return strings.TrimPrefix(strings.ToLower(sType), "secret_type_kms_")
}

// secretTypeFromString converts from flyctl's abbreviated secret type string form
// to the sType that flaps accepts.
func secretTypeFromString(s string) (string, error) {
	norm := secretTypeToString
	for _, typ := range validSecretTypes {
		if norm(s) == norm(typ) {
			return typ, nil
		}
	}

	validNames := []string{}
	for _, typ := range validSecretTypes {
		validNames = append(validNames, norm(typ))
	}
	return "", fmt.Errorf("invalid secret type. Must be one of %s", strings.Join(validNames, ", "))
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
