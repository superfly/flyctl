package buildinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/blang/semver"
)

var cachedName string // populated during init

func init() {
	var err error
	if cachedName, err = os.Executable(); err != nil {
		panic(err)
	}
	cachedName = filepath.Base(cachedName)
}

// Name returns the name for the executable that started the current
// process.
//
// Name is safe for concurrent use.
func Name() string {
	return cachedName
}

type info struct {
	Name         string
	Version      semver.Version
	Commit       string
	BuildDate    time.Time
	OS           string
	Architecture string
	Environment  string
}

func (i info) String() string {
	return fmt.Sprintf("%s v%s %s/%s Commit: %s BuildDate: %s",
		i.Name,
		i.Version,
		i.OS,
		i.Architecture,
		i.Commit,
		i.BuildDate.Format(time.RFC3339))
}

func Info() info {
	return info{
		Name:         Name(),
		Version:      Version(),
		Commit:       Commit(),
		BuildDate:    BuildDate(),
		OS:           OS(),
		Architecture: Arch(),
		Environment:  Environment(),
	}
}

func OS() string {
	return runtime.GOOS
}

func Arch() string {
	return runtime.GOARCH
}
