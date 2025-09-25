package appsecrets

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/superfly/fly-go"

	"github.com/superfly/flyctl/internal/flapsutil"
)

// List returns a list of app secrets. client must be a flaps client for appName.
// List will use the best known minvers for appName when listing secrets.
func List(ctx context.Context, client flapsutil.FlapsClient, appName string) ([]fly.AppSecret, error) {
	minver, err := GetMinvers(appName)
	if err != nil {
		return nil, err
	}
	return client.ListAppSecrets(ctx, minver, false)
}

// Update sets setSecrets and unsets unsetSecrets. client must be a flaps client for appName.
// It is not an error to unset a secret that does not exist.
// Update will keep track of the secrets minvers for appName after successfully changing secrets.
func Update(ctx context.Context, client flapsutil.FlapsClient, appName string, setSecrets map[string]string, unsetSecrets []string) error {
	update := map[string]*string{}
	for name, value := range setSecrets {
		value := value
		update[name] = &value
	}
	for _, name := range unsetSecrets {
		update[name] = nil
	}

	if len(update) == 0 {
		return nil
	}

	resp, err := client.UpdateAppSecrets(ctx, update)
	if err != nil {
		return err
	}

	if err := SetMinvers(ctx, appName, resp.Version); err != nil {
		return err
	}
	return nil
}

// Sync sets the min version for the app to the current min version, allowing
// any previously set secret to be visible in deploys.
func Sync(ctx context.Context, client flapsutil.FlapsClient, appName string) error {
	// This is somewhat of a hack -- we unset an non-existant secret and
	// we get back the latest min version after the unset.
	rand := make([]byte, 8)
	_, _ = crand.Read(rand)
	bogusDummySecret := fmt.Sprintf("BogusDummySecret_%s", hex.EncodeToString(rand))
	unsetSecrets := []string{bogusDummySecret}
	return Update(ctx, client, appName, nil, unsetSecrets)
}
