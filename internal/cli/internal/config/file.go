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
	return set(path, AccessTokenFileKey, token)
}

// ClearAccessToken unsets the value of the access token at the configuration
// file found at path.
func ClearAccessToken(path string) error {
	return unset(path, AccessTokenFileKey)
}

func unset(path, key string) error {
	var m map[string]interface{}

	switch err := unmarshal(path, &m); {
	case err == nil:
		break
	case os.IsNotExist(err):
		return nil // no file to unset from
	default:
		return nil
	}

	delete(m, key)

	return marshal(path, m)
}

func set(path, key string, value interface{}) error {
	var m map[string]interface{}
	switch err := unmarshal(path, &m); {
	case err == nil, os.IsNotExist(err):
		break
	default:
		return err
	}

	m[key] = value

	return marshal(path, m)
}

var lockPath = filepath.Join(os.TempDir(), "flyctl.config.lock")

func unmarshal(path string, into interface{}) (err error) {
	var unlocker filemu.Unlocker
	if unlocker, err = filemu.RLock(context.Background(), lockPath); err != nil {
		return
	}
	defer func() {
		if e := unlocker.Unlock(); err == nil {
			err = e
		}
	}()

	var f *os.File
	if f, err = os.Open(path); err != nil {
		return
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
	}()

	err = yaml.NewDecoder(f).Decode(&into)

	return
}

func marshal(path string, v interface{}) (err error) {
	var b bytes.Buffer
	if err = yaml.NewEncoder(&b).Encode(v); err == nil {
		return
	}

	var unlocker filemu.Unlocker
	if unlocker, err = filemu.Lock(context.Background(), lockPath); err != nil {
		return
	}
	defer func() {
		if e := unlocker.Unlock(); err == nil {
			err = e
		}
	}()

	// TODO: os.WriteFile does not flush
	err = os.WriteFile(path, b.Bytes(), 0600)

	return
}
