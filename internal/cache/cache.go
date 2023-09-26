// Package cache implements accessing of the state.yml file.
package cache

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/buildinfo"
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

	// SetCurrentVersionInvalid sets the current version of flyctl as invalid
	// because of the given error.
	SetCurrentVersionInvalid(err error)

	// IsCurrentVersionInvalid returns an error message if the given version
	// of flyctl is currently invalid. If not, it returns an empty string.
	IsCurrentVersionInvalid() string

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
	invalidVer    *invalidVer
}

func (c *cache) Channel() string {
	return update.NormalizeChannel(c.channel)
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

	if channel = update.NormalizeChannel(channel); c.channel != channel {
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

	if channel = update.NormalizeChannel(channel); channel != c.channel {
		return
	}

	c.dirty = true

	c.latestRelease = r
	c.lastCheckedAt = time.Now()
}

type invalidVer struct {
	Ver    string
	Reason string
}

func (c *cache) SetCurrentVersionInvalid(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dirty = true

	c.invalidVer = &invalidVer{Ver: buildinfo.Version().String(), Reason: err.Error()}
}

func (c *cache) IsCurrentVersionInvalid() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.invalidVer == nil {
		return ""
	}

	if c.invalidVer.Ver != buildinfo.Version().String() {
		return ""
	}

	return c.invalidVer.Reason
}

type wrapper struct {
	Channel       string          `yaml:"channel,omitempty"`
	LastCheckedAt time.Time       `yaml:"last_checked_at,omitempty"`
	LatestRelease *update.Release `yaml:"latest_release,omitempty"`
	InvalidVer    *invalidVer
}

func lockPath() string {
	return filepath.Join(flyctl.ConfigDir(), "flyctl.cache.lock")
}

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
		InvalidVer:    c.invalidVer,
	}
	if c.invalidVer != nil && c.IsCurrentVersionInvalid() == "" {
		w.InvalidVer = nil
	}

	if err = yaml.NewEncoder(&b).Encode(w); err != nil {
		return
	}

	var unlock filemu.UnlockFunc
	if unlock, err = filemu.Lock(context.Background(), lockPath()); err != nil {
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
	if unlock, err = filemu.RLock(context.Background(), lockPath()); err != nil {
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
			invalidVer:    w.InvalidVer,
		}
	}

	return
}
