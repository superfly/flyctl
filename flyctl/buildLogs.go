package flyctl

import (
	"strings"
	"time"

	"github.com/superfly/flyctl/api"
)

type BuildLogStream struct {
	client  *api.Client
	buildID string
	err     error
	build   *api.Build
}

func NewBuildLogStream(buildID string, client *api.Client) *BuildLogStream {
	return &BuildLogStream{client: client, buildID: buildID}
}

func (b *BuildLogStream) Err() error {
	return b.err
}

func (b *BuildLogStream) Status() string {
	return b.build.Status
}

func (b *BuildLogStream) Fetch() <-chan string {
	out := make(chan string, 0)

	go func() {
		defer close(out)

		pos := 0
		errors := 0
		for {
			build, err := b.client.GetBuild(b.buildID)
			if err != nil {
				errors++
				if errors < 3 {
					continue
				} else {
					b.err = err
					break
				}
			}
			errors = 0
			b.build = build

			lines := strings.Split(strings.TrimSpace(build.Logs), "\n")

			if len(lines) > pos {
				out <- strings.Join(lines[pos:], "\n")
				pos = len(lines)
			}

			if !build.InProgress {
				break
			}

			time.Sleep(1 * time.Second)
		}
	}()

	return out
}
