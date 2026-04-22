package image

import (
	"strings"

	"github.com/hashicorp/go-version"
	fly "github.com/superfly/fly-go"
)

// IsUpdateCandidate reports whether latest is a strictly newer build than the
// machine's current image. It guards against a backend resolver bug where
// asking for the latest details of a rolling tag (e.g. "flyio/postgres-flex:16")
// returns an older fly.version than the rolling tag's current one, which would
// otherwise surface as a spurious "update available" — actually a downgrade.
//
// When either fly.version label is missing or unparseable (e.g. custom images),
// falls back to a digest-inequality check so we preserve the prior behavior for
// cases that don't use semver-tagged builds.
func IsUpdateCandidate(machine *fly.Machine, latest *fly.ImageVersion) bool {
	if machine == nil || latest == nil {
		return false
	}
	if machine.ImageRef.Digest == latest.Digest {
		return false
	}

	current := strings.TrimPrefix(machine.ImageVersion(), "v")
	candidate := strings.TrimPrefix(latest.Version, "v")
	if current == "" || candidate == "" {
		return true
	}

	curV, err := version.NewVersion(current)
	if err != nil {
		return true
	}
	latV, err := version.NewVersion(candidate)
	if err != nil {
		return true
	}
	return latV.GreaterThan(curV)
}
