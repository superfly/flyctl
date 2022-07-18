package app

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAndSetEnvVariables(t *testing.T) {
	cfg := NewConfig()

	cfg.SetEnvVariable("A", "B")
	cfg.SetEnvVariable("C", "D")

	assert.Equal(t, map[string]string{"A": "B", "C": "D"}, cfg.GetEnvVariables())

	buf := &bytes.Buffer{}

	if err := cfg.EncodeTo(buf); err != nil {
		assert.NoError(t, err)
	}

	cfg2 := NewConfig()

	if err := cfg2.unmarshalTOML(bytes.NewReader(buf.Bytes())); err != nil {
		assert.NoError(t, err)
	}

	assert.Equal(t, cfg.GetEnvVariables(), cfg2.GetEnvVariables())
}
