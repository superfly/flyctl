package flyctl

import (
	"context"
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

func (b *BuildLogStream) Fetch(ctx context.Context) <-chan string {
	out := make(chan string, 0)

	go func() {
		defer close(out)

		pos := 0
		var interval time.Duration
		for {
			select {
			case <-time.After(interval):
				interval = 1 * time.Second
				build, err := fetchBuild(b.client, b.buildID)
				if err != nil {
					b.err = err
					return
				}
				b.build = build

				lines := strings.Split(strings.TrimSpace(build.Logs), "\n")

				if len(lines) > pos {
					out <- strings.Join(lines[pos:], "\n")
					pos = len(lines)
				}

				if !build.InProgress {
					return
				}
			case <-ctx.Done():
				b.err = ctx.Err()
				return
			}
		}
	}()

	return out
}

func fetchBuild(client *api.Client, buildID string) (build *api.Build, err error) {
	for attempts := 0; attempts < 3; attempts++ {
		build, err = client.GetBuild(buildID)
		if err == nil {
			break
		}
	}

	return build, err
}
