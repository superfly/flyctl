package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/oklog/ulid/v2"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		ctx       = context.TODO()
		apiClient = flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
			AccessToken: os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN"),
			BaseURL:     "https://api.fly.io",
			Logger:      logger.FromEnv(iostreams.System().ErrOut),
		})
	)
	if apiClient == nil {
		return fmt.Errorf("failed to init api client :-(")
	}
	_ = `# @genqlient
	query AllApps($orgSlug:String!) {
		organization(slug:$orgSlug) {
			apps {
				nodes {
					id
					createdAt
				}
			}
		}
	}`
	resp, err := gql.AllApps(ctx, apiClient.GenqClient(), os.Getenv("FLY_PREFLIGHT_TEST_FLY_ORG"))
	if err != nil {
		return err
	}
	for _, app := range resp.Organization.Apps.Nodes {
		if time.Since(app.CreatedAt) > 30*time.Minute {
			flyctlBin := "flyctl"
			cmdStr := fmt.Sprintf("%s apps destroy --yes %s", flyctlBin, app.Id)
			cmdParts, err := shlex.Split(cmdStr)
			if err != nil {
				return err
			}
			cmd := exec.CommandContext(ctx, flyctlBin, cmdParts[1:]...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
			cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
			cmd.Env = append(cmd.Env, os.Environ()...)
			cmd.Env = append(cmd.Env, fmt.Sprintf("FLY_API_TOKEN=%s", os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN")))
			fmt.Fprintln(os.Stderr, cmdStr)
			err = cmd.Start()
			if err != nil {
				return err
			}
			err = cmd.Wait()
			if err != nil {
				if isAppNotFound(stdout.String(), stderr.String()) {
					fmt.Fprintf(os.Stderr, "app %s is already gone; continuing cleanup\n", app.Id)
					continue
				}
				return err
			}
		}
	}

	return removeStalePeers(ctx, apiClient, os.Getenv("FLY_PREFLIGHT_TEST_FLY_ORG"))
}

// testPeerPrefix is the name prefix TestFlyWireguardCreate gives its peers,
// followed by the test env ULID, whose timestamp is the peer's creation time.
const testPeerPrefix = "test-peer-"

// removeStalePeers removes WireGuard peers left behind by preflight runs that
// were killed before their cleanup ran. The API has no creation time for
// peers, so it is read from the ULID in the name.
func removeStalePeers(ctx context.Context, apiClient *fly.Client, orgSlug string) error {
	org, err := apiClient.GetOrganizationBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}
	peers, err := apiClient.GetWireGuardPeers(ctx, orgSlug)
	if err != nil {
		return err
	}
	for _, peer := range peers {
		if !strings.HasPrefix(peer.Name, testPeerPrefix) {
			continue
		}
		id, err := ulid.ParseStrict(strings.TrimPrefix(peer.Name, testPeerPrefix))
		if err != nil {
			fmt.Fprintf(os.Stderr, "skipping peer %s: name does not end in a ULID: %v\n", peer.Name, err)
			continue
		}
		if time.Since(ulid.Time(id.Time())) < 30*time.Minute {
			continue
		}
		fmt.Fprintf(os.Stderr, "removing stale wireguard peer %s\n", peer.Name)
		if err := apiClient.RemoveWireGuardPeer(ctx, org.ID, peer.Name); err != nil {
			return fmt.Errorf("removing peer %s: %w", peer.Name, err)
		}
	}

	return nil
}

func isAppNotFound(stdout, stderr string) bool {
	output := strings.ToLower(stdout + "\n" + stderr)
	return strings.Contains(output, "app not found")
}
