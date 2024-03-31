package release

import (
	"time"

	"github.com/superfly/flyctl/internal/version"
)

type Meta struct {
	Version         *version.Version `json:"version"`
	Channel         string           `json:"channel"`
	Commit          string           `json:"commit"`
	CommitTime      time.Time        `json:"commit_time"`
	Tag             string           `json:"tag"`
	Branch          string           `json:"branch"`
	Dirty           bool             `json:"dirty"`
	Ref             string           `json:"ref"`
	PreviousVersion *version.Version `json:"previous_version"`
	PreviousTag     string           `json:"previous_tag"`
}
