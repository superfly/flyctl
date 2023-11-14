package state

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStrings(t *testing.T) {
	cases := map[string]struct {
		getter func(context.Context) string
		setter func(context.Context, string) context.Context
	}{
		"Hostname":         {Hostname, WithHostname},
		"WorkingDirectory": {WorkingDirectory, WithWorkingDirectory},
		"ConfigDirectory":  {ConfigDirectory, WithConfigDirectory},
	}

	for name := range cases {
		kase := cases[name]

		t.Run(fmt.Sprintf("%sPanics", name), func(t *testing.T) {
			assert.Panics(t, func() { _ = kase.getter(context.Background()) })
		})

		t.Run(fmt.Sprint(name), func(t *testing.T) {
			const exp = "expectation"

			ctx := kase.setter(context.Background(), exp)
			assert.Equal(t, exp, kase.getter(ctx))
		})
	}
}
