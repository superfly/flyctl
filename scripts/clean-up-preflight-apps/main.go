package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/google/shlex"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/buildinfo"
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
		apiClient = api.NewClientFromOptions(api.ClientOptions{
			AccessToken: os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN"),
			Name:        buildinfo.Name(),
			Version:     buildinfo.Version().String(),
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
	resp, err := gql.AllApps(ctx, apiClient.GenqClient, os.Getenv("FLY_PREFLIGHT_TEST_FLY_ORG"))
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
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Env = append(cmd.Env, os.Environ()...)
			cmd.Env = append(cmd.Env, fmt.Sprintf("FLY_API_TOKEN=%s", os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN")))
			fmt.Fprintln(os.Stderr, cmdStr)
			err = cmd.Start()
			if err != nil {
				return err
			}
			err = cmd.Wait()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
