// Package cache implements accessing of the state.yml file.
package cache

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/internal/update"
)

// FileName denotes the name of the cache file.
const FileName = "state.yml"

// Cache wraps the functionality of the local cache file.
type Cache interface {
	// Dirty reports whether the cache has been mutated since being loaded
	// or initialized.
	Dirty() bool

	// Channel reports the release channel the cache is subscribed to.
	Channel() string

	// SetChannel sets the channel to the one corresponding to the given
	// string. It reports the value the channel was actually set to.
	//
	// Calling SetChannel on a Cache set to a different channel, zeroes out
	// the Cache's release & timestamp information.
	SetChannel(string) string

	// LastCheckedAt reports the last time SetLatestRelease with a non-nil
	// value was called on the Cache.
	LastCheckedAt() time.Time

	// LatestRelease reports the latest release the cache is aware of.
	LatestRelease() *update.Release

	// SetLatestRelease sets the latest release for the given channel.
	//
	// Calling SetLatestRelease for a different channel than the one the Cache
	// is set to has no effect.
	SetLatestRelease(channel string, r *update.Release)

	// Save writes the YAML-encoded representation of c to the named file path via
	// os.WriteFile.
	Save(path string) error
}

const defaultChannel = "latest"

// New initializes and returns a reference to a new cache.
func New() Cache {
	return &cache{
		channel: defaultChannel,
	}
}

type cache struct {
	mu            sync.RWMutex // protects below
	dirty         bool
	channel       string
	lastCheckedAt time.Time
	latestRelease *update.Release
}

func (c *cache) Channel() string {
	return normalizeChannel(c.channel)
}

func normalizeChannel(c string) string {
	const pre = "pre"
	if strings.Contains(c, pre) {
		return pre
	}

	return "latest"
}

func (c *cache) Dirty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.dirty
}

func (c *cache) SetChannel(channel string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dirty = true

	if channel = normalizeChannel(channel); c.channel != channel {
		// purge timestamp & release since we're changing channels
		c.lastCheckedAt = time.Time{}
		c.latestRelease = nil
	}

	c.channel = channel

	return c.channel
}

func (c *cache) LastCheckedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lastCheckedAt
}

func (c *cache) LatestRelease() *update.Release {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.latestRelease
}

func (c *cache) SetLatestRelease(channel string, r *update.Release) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if channel = normalizeChannel(channel); channel != c.channel {
		return
	}

	c.dirty = true

	c.latestRelease = r
	c.lastCheckedAt = time.Now()
}

type wrapper struct {
	Channel       string          `yaml:"channel,omitempty"`
	LastCheckedAt time.Time       `yaml:"last_checked_at,omitempty"`
	LatestRelease *update.Release `yaml:"latest_release,omitempty"`
}

var lockPath = filepath.Join(os.TempDir(), "flyctl.cache.lock")

// Save writes the YAML-encoded representation of c to the named file path via
// os.WriteFile.
func (c *cache) Save(path string) (err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var b bytes.Buffer

	w := wrapper{
		Channel:       c.channel,
		LastCheckedAt: c.lastCheckedAt,
		LatestRelease: c.latestRelease,
	}

	if err = yaml.NewEncoder(&b).Encode(w); err != nil {
		return
	}

	var unlock filemu.UnlockFunc
	if unlock, err = filemu.Lock(context.Background(), lockPath); err != nil {
		return
	}
	defer func() {
		if e := unlock(); err == nil {
			err = e
		}
	}()

	// TODO: os.WriteFile does NOT flush
	err = os.WriteFile(path, b.Bytes(), 0o600)

	return
}

// Load loads the YAML-encoded cache file at the given path.
func Load(path string) (c Cache, err error) {
	var unlock filemu.UnlockFunc
	if unlock, err = filemu.RLock(context.Background(), lockPath); err != nil {
		return
	}
	defer func() {
		if e := unlock(); err == nil {
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

	var w wrapper
	if err = yaml.NewDecoder(f).Decode(&w); err == nil {
		c = &cache{
			channel:       w.Channel,
			lastCheckedAt: w.LastCheckedAt,
			latestRelease: w.LatestRelease,
		}
	}

	return
}
