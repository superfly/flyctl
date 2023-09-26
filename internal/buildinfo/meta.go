package buildinfo

import "time"

//go:generate go run gen.go

type Meta struct {
	Channel   string
	Version   string
	BuildTime time.Time
	GitCommit string
	GitBranch string
	GitDirty  bool
}
