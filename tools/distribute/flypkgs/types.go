package flypkgs

import (
	"time"

	"github.com/superfly/flyctl/internal/version"
)

type Release struct {
	ID             uint64          `json:"id"`
	Channel        Channel         `json:"channel"`
	Version        version.Version `json:"version"`
	GitCommit      string          `json:"git_commit"`
	GitBranch      string          `json:"git_branch"`
	GitTag         string          `json:"git_tag"`
	GitPreviousTag string          `json:"git_previous_tag"`
	GitDirty       bool            `json:"git_dirty"`
	Status         string          `json:"status"`
	InsertedAt     time.Time       `json:"inserted_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	PublishedAt    time.Time       `json:"published_at"`
	Assets         []Asset         `json:"assets"`
}

type Channel struct {
	ID         uint64    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Stable     bool      `json:"stable"`
	InsertedAt time.Time `json:"inserted_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Asset struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Size        uint64 `json:"size"`
	SHA256      string `json:"sha256"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	ContentType string `json:"content_type"`
	InsertedAt  string `json:"inserted_at"`
	UpdatedAt   string `json:"updated_at"`
}
