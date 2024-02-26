package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
)

const (
	execInfoEnv = "KUBERNETES_EXEC_INFO"
	tokenPrefix = "FlyV1 "
)

type response struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Status     status `json:"status"`
}

type status struct {
	Token      string `json:"token,omitempty"`
	Expiration string `json:"expirationTimestamp,omitempty"`
}

type auth struct {
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
	cmd.Hidden = true

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

	configDir, err := helpers.GetConfigDirectory()
	if err != nil {
		fmt.Println("Error accessing home directory", err)
		return err
	}

	fksConfigDir := filepath.Join(configDir, "fks", orgSlug)
	configPath := filepath.Join(fksConfigDir, "config.yml")

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	var token string
	var expiry int64
	now := time.Now().UTC()

	switch err := v.ReadInConfig(); err {
	case nil:
		fmt.Fprintf(os.Stderr, "Using existing token")
		token = v.GetString("auth.token")
		expiry = int64(v.GetInt("auth.expiration"))
		if time.Now().Unix() > expiry {
			fmt.Fprintf(os.Stderr, "Token expired, generating new token")
			tokenResp, err := makeOrgToken(ctx, client, org.ID, (time.Hour).String())
			if err != nil {
				return err
			}

			token = tokenResp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
			token = strings.TrimPrefix(token, tokenPrefix)
			expiry = now.Add(time.Hour).Unix()
		}
	default:
		fmt.Fprintf(os.Stderr, "No existing token, generating new one for the first time")
		// path doesn't exist, create the path
		if !helpers.DirectoryExists(fksConfigDir) {
			if err := os.MkdirAll(fksConfigDir, 0o700); err != nil {
				return err
			}
		}
		tokenResp, err := makeOrgToken(ctx, client, org.ID, (time.Hour).String())
		if err != nil {
			return err
		}

		token = tokenResp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
		token = strings.TrimPrefix(token, tokenPrefix)
		expiry = now.Add(time.Hour).Unix()
	}

	v.Set("auth.token", token)
	v.Set("auth.expiration", expiry)
	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("could not write fks config file (error: %s)", err)
	}

	resp.Status.Token = token
	resp.Status.Expiration = time.Unix(expiry, 0).Format(time.RFC3339Nano)

	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(resp); err != nil {
		return err
	}

	fmt.Fprint(os.Stderr, buffer.String())
	fmt.Println(buffer.String())
	return nil
}

func makeOrgToken(ctx context.Context, apiClient *fly.Client, orgID string, expiration string) (*gql.CreateLimitedAccessTokenResponse, error) {
	options := gql.LimitedAccessTokenOptions{}
	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient,
		"FKS org deploy token",
		orgID,
		"deploy_organization",
		&options,
		expiration,
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating deploy token: %w", err)
	}
	return resp, nil
}
