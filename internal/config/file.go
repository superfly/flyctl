package config

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/superfly/flyctl/internal/filemu"
)

// SetAccessToken sets the value of the access token at the configuration file
// found at path.
func SetAccessToken(path, token string) error {
	return set(path, map[string]interface{}{
		AccessTokenFileKey: token,
	})
}

// Clear clears the access token and wireguard-related keys of the configuration
// file found at path.
func Clear(path string) (err error) {
	return set(path, map[string]interface{}{
		AccessTokenFileKey:    "",
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

var lockPath = filepath.Join(os.TempDir(), "flyctl.config.lock")

func unmarshal(path string, v interface{}) (err error) {
	var unlock filemu.UnlockFunc
	if unlock, err = filemu.RLock(context.Background(), lockPath); err != nil {
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

func marshal(path string, v interface{}) (err error) {
	var unlock filemu.UnlockFunc
	if unlock, err = filemu.Lock(context.Background(), lockPath); err != nil {
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
