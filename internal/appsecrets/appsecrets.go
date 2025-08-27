package appsecrets

import (
	"context"

	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/state"
)

func getMinvers() (config.AppSecretsMinvers, error) {
	minvers := config.AppSecretsMinvers{}
	if err := viper.UnmarshalKey(flyctl.ConfigAppSecretsMinvers, &minvers); err != nil {
		return nil, errors.Wrap(err, "invalid application secrets minversions")
	}
	return minvers, nil
}

// GetAppSecretsMinvers returns the minimum secrets version for appID if known or nil.
func GetAppSecretsMinvers(appID string) (*uint64, error) {
	minvers, err := getMinvers()
	if err != nil {
		return nil, err
	}

	if v, ok := minvers[appID]; ok {
		return &v, nil
	}
	return nil, nil
}

func setAppSecretsMinvers(ctx context.Context, appID string, v *uint64) error {
	minvers, err := getMinvers()
	if err != nil {
		return err
	}

	if v == nil {
		delete(minvers, appID)
	} else {
		minvers[appID] = *v
	}

	viper.Set(flyctl.ConfigAppSecretsMinvers, minvers)
	configPath := state.ConfigFile(ctx)
	if err := config.SetAppSecretsMinvers(configPath, minvers); err != nil {
		return errors.Wrap(err, "error saving config file")
	}

	return nil
}

// SetAppSecretsMinvers sets the minimum secrets version for appID and saves it.
func SetAppSecretsMinvers(ctx context.Context, appID string, v uint64) error {
	return setAppSecretsMinvers(ctx, appID, &v)
}

// DeleteAppSecretsMinvers removes the minimum secrets version for appID and saves it.
func DeleteAppSecretsMinvers(ctx context.Context, appID string) error {
	return setAppSecretsMinvers(ctx, appID, nil)
}
