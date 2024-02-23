package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
)

const execInfoEnv = "KUBERNETES_EXEC_INFO"

type response struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Status     status `json:"status"`
}

type status struct {
	Token      string `json:"token,omitempty"`
	Expiration string `json:"expirationTimestamp,omitempty"`
}

type PartialExecCredential struct {
	Spec struct {
		Cluster struct {
			Config map[string]string `json:"config"`
		} `json:"cluster"`
	} `json:"spec"`
}

func kubectlToken() (cmd *cobra.Command) {
	const (
		long  = `Get an authentication token for your Kubernetes clusters`
		short = long
		usage = "kubectl-token"
	)

	cmd = command.New(usage, short, long, runAuth, command.RequireSession)
	cmd.Hidden = false

	return cmd
}

func runAuth(ctx context.Context) error {
	var (
		client = fly.ClientFromContext(ctx)
		resp   = response{
			APIVersion: "client.authentication.k8s.io/v1",
			Kind:       "ExecCredential",
		}
	)

	execInfo := os.Getenv(execInfoEnv)
	if execInfo == "" {
		return errors.New("KUBERNETES_EXEC_INFO env var is unset or empty")
	}

	var execCredential PartialExecCredential
	err := json.Unmarshal([]byte(execInfo), &execCredential)
	if err != nil {
		return err
	}

	orgSlug := execCredential.Spec.Cluster.Config["org"]
	org, err := orgs.OrgFromSlug(ctx, orgSlug)
	if err != nil {
		return fmt.Errorf("could not find org id for org %s", orgSlug)
	}

	tokenResp, err := makeOrgToken(ctx, client, org.ID)
	if err != nil {
		return err
	}

	resp.Status.Token = tokenResp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(resp); err != nil {
		return err
	}

	fmt.Println(buffer.String())
	return nil
}

func makeOrgToken(ctx context.Context, apiClient *fly.Client, orgID string) (*gql.CreateLimitedAccessTokenResponse, error) {
	options := gql.LimitedAccessTokenOptions{}
	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient,
		"FKS org deploy token",
		orgID,
		"deploy_organization",
		&options,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating deploy token: %w", err)
	}
	return resp, nil
}
