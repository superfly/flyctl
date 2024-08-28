package config

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/superfly/flyctl/wg"
	"gopkg.in/yaml.v3"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/filemu"
)

func ReadAccessToken(path string) (string, error) {
	s := struct {
		AccessToken string `yaml:"access_token"`
	}{}
	if err := unmarshal(path, &s); err != nil {
		return "", err
	}

	return s.AccessToken, nil
}

// SetAccessToken sets the value of the access token at the configuration file
// found at path.
func SetAccessToken(path, token string) error {
	return set(path, map[string]interface{}{
		AccessTokenFileKey: token,
	})
}

// SetMetricsToken sets the value of the metrics token at the configuration file
// found at path.
func SetMetricsToken(path, token string) error {
	return set(path, map[string]interface{}{
		MetricsTokenFileKey: token,
	})
}

// SetSendMetrics sets the value of the send metrics flag at the configuration file
// found at path.
func SetSendMetrics(path string, sendMetrics bool) error {
	return set(path, map[string]interface{}{
		SendMetricsFileKey: sendMetrics,
	})
}

// SetSyntheticsAgent sets the value of the synthetics agent flag at the configuration file
// found at path.
func SetSyntheticsAgent(path string, syntheticsAgent bool) error {
	return set(path, map[string]interface{}{
		SyntheticsAgentFileKey: syntheticsAgent,
	})
}

// SetAutoUpdate sets the value of the autoupdate flag at the configuration file
// found at path.
func SetAutoUpdate(path string, autoUpdate bool) error {
	return set(path, map[string]interface{}{
		AutoUpdateFileKey: autoUpdate,
	})
}

func SetWireGuardState(path string, state wg.States) error {
	return set(path, map[string]interface{}{
		WireGuardStateFileKey: state,
	})
}

func SetWireGuardWebsocketsEnabled(path string, enabled bool) error {
	return set(path, map[string]interface{}{
		WireGuardWebsocketsFileKey: enabled,
	})
}

// Clear clears the access token, metrics token, and wireguard-related keys of the configuration
// file found at path.
func Clear(path string) (err error) {
	return set(path, map[string]interface{}{
		AccessTokenFileKey:    "",
		MetricsTokenFileKey:   "",
		WireGuardStateFileKey: map[string]interface{}{},
	})
}

func set(path string, vals map[string]interface{}) error {
	m := make(map[string]interface{})

	switch err := unmarshal(path, &m); {
	case err == nil, os.IsNotExist(err):
		break
	default:
		return err
	}

	for k, v := range vals {
		m[k] = v
	}

	return marshal(path, m)
}

func lockPath() string {
	return filepath.Join(flyctl.ConfigDir(), "flyctl.config.lock")
}

func unmarshal(path string, v interface{}) (err error) {
	var unlock filemu.UnlockFunc
	if unlock, err = filemu.RLock(context.Background(), lockPath()); err != nil {
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
	if err == io.EOF {
		err = nil
	}

	return
}

func marshal(path string, v interface{}) (err error) {
	var unlock filemu.UnlockFunc
	if unlock, err = filemu.Lock(context.Background(), lockPath()); err != nil {
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
