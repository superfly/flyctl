package config

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
)

func TestConfigWatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = logger.NewContext(ctx, logger.New(io.Discard, logger.Error, false))
	ctx = flagctx.NewContext(ctx, new(pflag.FlagSet))

	tm := task.New()
	tm.Start(ctx)
	ctx = task.WithContext(ctx, tm)

	path := path.Join(t.TempDir(), "config.yml")

	require.NoError(t, os.WriteFile(path, []byte(`access_token: fo1_foo`), 0644))
	cfg, err := Load(ctx, path)
	require.NoError(t, err)
	require.Equal(t, "fo1_foo", cfg.Tokens.All())

	c1, err := cfg.Watch(ctx)
	require.NoError(t, err)

	c2, err := cfg.Watch(ctx)
	require.NoError(t, err)

	cfgs, errs := getConfigChanges(c1, c2)
	require.Equal(t, 2, len(errs))
	require.Equal(t, 0, len(cfgs))

	require.NoError(t, os.WriteFile(path, []byte(`access_token: fo1_bar`), 0644))

	cfgs, errs = getConfigChanges(c1, c2)
	require.Equal(t, 0, len(errs), errs)
	require.Equal(t, 2, len(cfgs))
	require.Equal(t, cfgs[0], cfgs[1])
	require.Equal(t, "fo1_bar", cfgs[0].Tokens.All())

	// debouncing
	require.NoError(t, os.WriteFile(path, []byte(`access_token: fo1_aaa`), 0644))
	require.NoError(t, os.WriteFile(path, []byte(`access_token: fo1_bbb`), 0644))

	cfgs, errs = getConfigChanges(c1, c2)
	require.Equal(t, 0, len(errs))
	require.Equal(t, 2, len(cfgs))
	require.Equal(t, cfgs[0], cfgs[1])
	require.Equal(t, "fo1_bbb", cfgs[0].Tokens.All())

	cfgs, errs = getConfigChanges(c1, c2)
	require.Equal(t, 2, len(errs))
	require.Equal(t, 0, len(cfgs))

	cfg.Unwatch(c1)

	require.NoError(t, os.WriteFile(path, []byte(`access_token: fo1_baz`), 0644))

	cfgs, errs = getConfigChanges(c2)
	require.Equal(t, 0, len(errs))
	require.Equal(t, 1, len(cfgs))
	require.Equal(t, "fo1_baz", cfgs[0].Tokens.All())

	shutdown := make(chan struct{})
	go func() {
		defer close(shutdown)
		tm.Shutdown()
	}()
	select {
	case <-shutdown:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("slow shutdown")
	}

	_, open := <-c1
	require.False(t, open)
	_, open = <-c2
	require.False(t, open)

	_, err = cfg.Watch(ctx)
	assert.Error(t, err)
	require.EqualError(t, err, context.Canceled.Error())
}

func getConfigChanges(chans ...chan *Config) ([]*Config, []error) {
	var (
		cfgs []*Config
		errs []error
		m    sync.Mutex
		wg   sync.WaitGroup
	)

	for _, ch := range chans {
		ch := ch

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer m.Unlock()

			select {
			case cfg, open := <-ch:
				m.Lock()
				if open {
					cfgs = append(cfgs, cfg)
				} else {
					errs = append(errs, errors.New("closed"))
				}
			case <-time.After(100 * time.Millisecond):
				m.Lock()
				errs = append(errs, errors.New("timeout"))
			}
		}()
	}

	wg.Wait()

	return cfgs, errs
}
