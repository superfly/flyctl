// Package doctor implements the doctor command chain.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"

	docker "github.com/docker/docker/client"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

// New initializes and returns a new doctor Command.
func New() (cmd *cobra.Command) {
	const (
		short = `The DOCTOR command allows you to debug your Fly environment`
		long  = short + "\n"
	)

	cmd = command.New("doctor", short, long, run,
		command.RequireSession,
	)

	return
}

var runners = map[string]runner{
	"Token":          runAuth,
	"Docker (local)": runLocalDocker,
	"Agent":          runAgent,
	"UDP":            runUDP,
}

func run(ctx context.Context) error {
	errors := runInParallel(ctx, runtime.GOMAXPROCS(0), runners)

	if err := ctx.Err(); err != nil {
		return err
	}

	if out := iostreams.FromContext(ctx).Out; config.FromContext(ctx).JSONOutput {
		return render.JSON(out, errors)
	}

	return renderTable(ctx, errors)
}

type limiter chan struct{}

func (l limiter) acquire() { l <- struct{}{} }

func (l limiter) relinquish() { <-l }

type runner func(context.Context) error

func runInParallel(ctx context.Context, concurrency int, runners map[string]runner) map[string]error {
	l := make(limiter, concurrency)

	var mu sync.Mutex
	ret := make(map[string]error, len(runners))

	var wg sync.WaitGroup
	wg.Add(len(runners))

	for key := range runners {
		go func(key string) {
			defer wg.Done()

			l.acquire()
			defer l.relinquish()

			err := runners[key](ctx)

			mu.Lock()
			defer mu.Unlock()

			ret[key] = err
		}(key)
	}

	wg.Wait()

	return ret
}

func renderTable(ctx context.Context, errors map[string]error) error {
	keys := make([]string, 0, len(errors))
	for key := range errors {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rows := make([][]string, 0, len(errors))
	for _, key := range keys {
		rows = append(rows, []string{
			key,
			toReason(errors[key]),
		})
	}

	out := iostreams.FromContext(ctx).Out

	return render.Table(out, "", rows, "Test", "Status")
}

func toReason(err error) string {
	if err == nil {
		return aurora.Green("PASS").String()
	}

	return aurora.Red(err.Error()).String()
}

func runAuth(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()

	if _, err = client.GetCurrentUser(ctx); err != nil {
		err = errors.New("your access token is not valid; use `flyctl auth login` to login again")
	}

	return
}

func runLocalDocker(ctx context.Context) (err error) {
	defer func() {
		if err == nil {
			return
		}

		err = fmt.Errorf("couldn't ping local docker instance: %w", err)
	}()

	var c *docker.Client
	if c, err = docker.NewClientWithOpts(docker.WithAPIVersionNegotiation()); err != nil {
		return
	}

	if err = docker.FromEnv(c); err == nil {
		_, err = c.Ping(ctx)
	}

	return
}

func runAgent(ctx context.Context) (err error) {
	defer func() {
		if err == nil {
			return
		}

		err = fmt.Errorf("couldn't ping agent: %w", err)
	}()

	client := client.FromContext(ctx).API()

	var ac *agent.Client
	if ac, err = agent.DefaultClient(client); err == nil {
		_, err = ac.Ping(ctx)
	}

	return
}

func runUDP(ctx context.Context) error {
	return errors.New("server not implemented yet")
}
