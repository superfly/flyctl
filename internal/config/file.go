package config

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/wg"
)

// SetAccessToken sets the value of the access token at the configuration file
// contained in ctx.
func SetAccessToken(ctx context.Context, token string) error {
	return set(ctx, map[string]interface{}{
		AccessTokenFileKey: token,
	})
}

// SetMetricsToken sets the value of the metrics token at the configuration file
// contained in ctx.
func SetMetricsToken(ctx context.Context, token string) error {
	return set(ctx, map[string]interface{}{
		MetricsTokenFileKey: token,
	})
}

// SetSendMetrics sets the value of the send metrics flag at the configuration file
// contained in ctx.
func SetSendMetrics(ctx context.Context, sendMetrics bool) error {
	return set(ctx, map[string]interface{}{
		SendMetricsFileKey: sendMetrics,
	})
}

// SetAutoUpdate sets the value of the autoupdate flag at the configuration file
// contained in ctx.
func SetAutoUpdate(ctx context.Context, autoUpdate bool) error {
	return set(ctx, map[string]interface{}{
		AutoUpdateFileKey: autoUpdate,
	})
}

func SetWireGuardState(ctx context.Context, state map[string]*wg.WireGuardState) error {
	return set(ctx, map[string]interface{}{
		WireGuardStateFileKey: state,
	})
}

func SetWireGuardWebsocketsEnabled(ctx context.Context, enabled bool) error {
	return set(ctx, map[string]interface{}{
		WireGuardWebsocketsFileKey: enabled,
	})
}

// Clear clears the access token, metrics token, and wireguard-related keys of the configuration
// file contained in ctx.
func Clear(ctx context.Context) (err error) {
	return set(ctx, map[string]interface{}{
		AccessTokenFileKey:    "",
		MetricsTokenFileKey:   "",
		WireGuardStateFileKey: map[string]interface{}{},
	})
}

func set(ctx context.Context, vals map[string]interface{}) error {
	m := make(map[string]interface{})

	switch err := unmarshal(ctx, &m); {
	case err == nil, os.IsNotExist(err):
		break
	default:
		return err
	}

	for k, v := range vals {
		m[k] = v
	}

	return marshal(ctx, m)
}

func lockPath(ctx context.Context) string {
	return filepath.Join(state.RuntimeDirectory(ctx), "flyctl.lock")
}

func unmarshal(ctx context.Context, v interface{}) (err error) {
	var unlock filemu.UnlockFunc
	var path = filepath.Join(state.ConfigDirectory(ctx), FileName)
	if unlock, err = filemu.RLock(context.Background(), lockPath(ctx)); err != nil {
		return
	}
	defer func() {
		if e := unlock(); err == nil {
			err = e
		}
	}()

	err = unmarshalUnlocked(path, v)

	return
}

func unmarshalUnlocked(path string, v interface{}) (err error) {
	var f *os.File
	if f, err = os.Open(path); err != nil {
		return
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
	}()

	err = yaml.NewDecoder(f).Decode(v)

	return
}

func marshal(ctx context.Context, v interface{}) (err error) {
	var unlock filemu.UnlockFunc
	var path = filepath.Join(state.ConfigDirectory(ctx), FileName)
	if unlock, err = filemu.Lock(context.Background(), lockPath(ctx)); err != nil {
		return
	}
	defer func() {
		if e := unlock(); err == nil {
			err = e
		}
	}()

	err = marshalUnlocked(path, v)

	return
}

func marshalUnlocked(path string, v interface{}) (err error) {
	var b bytes.Buffer
	if err = yaml.NewEncoder(&b).Encode(v); err == nil {
		// TODO: os.WriteFile does not flush
		err = os.WriteFile(path, b.Bytes(), 0o600)
	}

	return
}
