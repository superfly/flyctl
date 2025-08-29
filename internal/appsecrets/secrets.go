package appsecrets

import (
	"context"

	"github.com/superfly/fly-go"

	"github.com/superfly/flyctl/internal/flapsutil"
)

// List returns a list of app secrets. client must be a flaps client for appName.
// List will use the best known minvers for appName when listing secrets.
func List(ctx context.Context, client flapsutil.FlapsClient, appName string) ([]fly.AppSecret, error) {
	minver, err := GetAppSecretsMinvers(appName)
	if err != nil {
		return nil, err
	}
	return client.ListAppSecrets(ctx, minver, false)
}

// Update sets setSecrets and unsets unsetSecrets. client must be a flaps client for appName.
// It is not an error to unset a secret that does not exist.
// Update will keep track of the secrets minvers for appName after successfully changing secrets.
func Update(ctx context.Context, client flapsutil.FlapsClient, appName string, setSecrets map[string]string, unsetSecrets []string) error {
	var update map[string]*string
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

	if err := SetAppSecretsMinvers(ctx, appName, resp.Version); err != nil {
		return err
	}
	return nil
}
