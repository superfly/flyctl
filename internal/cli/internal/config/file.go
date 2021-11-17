package config

import (
	"bytes"
	"os"

	"gopkg.in/yaml.v3"
)

// Unset unsets the key of the configuration file at the named path.
func Unset(path, key string) error {
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

// Set sets the key of the configuration file at the named path to value.
func Set(path, key string, value interface{}) error {
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

// TODO: this is prone to race conditions
func unmarshal(path string, into interface{}) (err error) {
	var f *os.File
	if f, err = os.Open(path); err == nil {
		err = yaml.NewDecoder(f).Decode(&into)

		if e := f.Close(); err == nil {
			err = e
		}
	}

	return
}

// TODO: this is prone to race conditions and os.WriteFile does not flush
func marshal(path string, v interface{}) (err error) {
	var b bytes.Buffer

	if err = yaml.NewEncoder(&b).Encode(v); err == nil {
		err = os.WriteFile(path, b.Bytes(), 0600)
	}

	return
}
