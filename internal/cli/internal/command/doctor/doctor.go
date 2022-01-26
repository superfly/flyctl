// Package doctor implements the doctor command chain.
package doctor

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/azazeal/pause"
	dockerclient "github.com/docker/docker/client"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
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
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

var runners = map[string]runner{
	"Token":          runAuth,
	"Docker (local)": runLocalDocker,
	"Agent":          runAgent,
	"Probe (app)":    runProbeApp,
	"Unix socket":    runUnixSocket,
	// "UDP":            runUDP,
}

func run(ctx context.Context) error {
	errors := runInParallel(ctx, runtime.GOMAXPROCS(0), runners)

	if err := ctx.Err(); err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		return renderJSON(ctx, errors)
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

			if !errors.Is(err, errSkipped) {
				ret[key] = err
			}
		}(key)
	}

	wg.Wait()

	return ret
}

func renderJSON(ctx context.Context, errors map[string]error) error {
	m := make(map[string]string, len(errors))

	for k, err := range errors {
		if err == nil {
			m[k] = ""

			continue
		}

		m[k] = err.Error()
	}

	out := iostreams.FromContext(ctx).Out
	return render.JSON(out, m)
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

		err = fmt.Errorf("failed pinging docker instance: %w", err)
	}()

	var client *dockerclient.Client
	if client, err = imgsrc.NewLocalDockerClient(); err == nil {
		_, err = client.Ping(ctx)
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

	var ac *agent.Client
	if ac, err = agent.DefaultClient(ctx); err == nil {
		_, err = ac.Ping(ctx)
	}

	return
}

var errSkipped = errors.New("skipped")

func runProbeApp(ctx context.Context) (err error) {
	appName := app.NameFromContext(ctx)
	if appName == "" {
		return errSkipped
	}

	client := client.FromContext(ctx).API()

	var app *api.App
	if app, err = client.GetApp(ctx, appName); err != nil {
		err = fmt.Errorf("failed retrieving app: %w", err)

		return
	}

	var ac *agent.Client
	if ac, err = agent.Establish(ctx, client); err != nil {
		err = fmt.Errorf("failed establishing agent connection: %w", err)

		return
	}

	slug := app.Organization.Slug
	if _, err = ac.Establish(ctx, slug); err != nil {
		err = fmt.Errorf("failed establishing tunnel to %s: %w", slug, err)

		return
	}

	if err = ac.Probe(ctx, slug); err != nil {
		err = fmt.Errorf("failed probing %s: %w", slug, err)
	}

	return
}

func runUDP(ctx context.Context) error {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return fmt.Errorf("failed seeding: %w", err)
	}

	const addr = "debug.fly.dev:10000"

	conn, err := net.Dial("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed dialing %s: %w", addr, err)
	}
	defer conn.Close()

	const interval = 50 * time.Millisecond

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		for i := 0; i < 10 && ctx.Err() == nil; i++ {
			if _, err := conn.Write(seed); err != nil {
				return fmt.Errorf("failed writing: %w", err)
			}

			pause.For(ctx, interval)
		}

		return nil
	})

	eg.Go(func() error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		buf := make([]byte, len(seed))

		for ctx.Err() == nil {
			dl := time.Now().Add(interval)
			if err := conn.SetReadDeadline(dl); err != nil {
				return fmt.Errorf("failed setting read deadline: %w", err)
			}

			switch n, err := conn.Read(buf); {
			case isNetworkTimeout(err):
				break
			case err != nil:
				return fmt.Errorf("failed reading: %w", err)
			case bytes.Equal(seed, buf[:n]):
				return nil
			}
		}

		return errors.New("no UDP connectivity detected")
	})

	return eg.Wait()
}

func isNetworkTimeout(err error) bool {
	e, ok := err.(net.Error)
	return ok && e.Timeout()
}

func runUnixSocket(ctx context.Context) error {
	path := filepath.Join(os.TempDir(), "fly-doctor.socket")

	l, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("failed listening on socket: %w", err)
	}

	if err := l.Close(); err != nil {
		return fmt.Errorf("failed closing socket: %w", err)
	}

	return nil
}
