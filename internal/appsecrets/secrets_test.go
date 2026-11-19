package appsecrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/state"
)

// Launch, the attach commands and `secrets sync` print Update's error as it
// is. A failed version save must keep the text those commands always printed,
// while staying detectable for callers that want to say more.
func TestUpdateVersionSaveErrorKeepsItsText(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // the config lock file lands in the working directory under tests
	previous := viper.Get(flyctl.ConfigAppSecretsMinvers)
	t.Cleanup(func() { viper.Set(flyctl.ConfigAppSecretsMinvers, previous) })
	viper.Set(flyctl.ConfigAppSecretsMinvers, config.AppSecretsMinvers{})
	// A directory in place of config.yml fails even when tests run as root.
	require.NoError(t, os.Mkdir(filepath.Join(dir, config.FileName), 0700))
	ctx := state.WithConfigDirectory(t.Context(), dir)
	client := &mock.FlapsClient{
		UpdateAppSecretsFunc: func(context.Context, string, map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			return &fly.UpdateAppSecretsResp{Version: 42}, nil
		},
	}

	wantErr := SetMinvers(ctx, "selected-app", 42)
	require.ErrorContains(t, wantErr, "error saving config file")

	for name, update := range map[string]func() error{
		"Update": func() error { return Update(ctx, client, "selected-app", map[string]string{"KEY": "value"}, nil) },
		"Sync":   func() error { return Sync(ctx, client, "selected-app") },
	} {
		t.Run(name, func(t *testing.T) {
			err := update()
			require.EqualError(t, err, wantErr.Error())
			var saveErr *VersionSaveError
			require.ErrorAs(t, err, &saveErr)
		})
	}
}

func TestUpdateRemoteFailureIsNotAVersionSaveError(t *testing.T) {
	denied := errors.New("permission denied")
	client := &mock.FlapsClient{
		UpdateAppSecretsFunc: func(context.Context, string, map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			return nil, denied
		},
	}
	err := Update(t.Context(), client, "selected-app", nil, []string{"KEY"})
	require.ErrorIs(t, err, denied)
	var saveErr *VersionSaveError
	require.False(t, errors.As(err, &saveErr))
}
