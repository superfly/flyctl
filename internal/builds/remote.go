package builds

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/superfly/flyctl/api"
)

type BuildMonitor struct {
	client  *api.Client
	buildID string
	err     error
	build   *api.Build
}

func NewBuildMonitor(buildID string, client *api.Client) *BuildMonitor {
	return &BuildMonitor{client: client, buildID: buildID}
}

func (b *BuildMonitor) Err() error {
	return b.err
}

func (b *BuildMonitor) Build() *api.Build {
	return b.build
}

func (b *BuildMonitor) Status() string {
	return b.build.Status
}

func (b *BuildMonitor) Failed() bool {
	return b.Status() == "failed"
}

func (b *BuildMonitor) Logs(ctx context.Context) <-chan string {
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
					for _, line := range lines[pos:] {
						out <- cleanLogLine(line)
						// out <- strings.Join(cleanLogLine(line), "\n")
					}
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

var timestampPrefixPattern = regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d{2}:\d{2}\]\s`)

func cleanLogLine(line string) string {
	return timestampPrefixPattern.ReplaceAllString(line, "")
}
