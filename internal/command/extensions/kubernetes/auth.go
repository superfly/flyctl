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
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flyutil"
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

type PartialExecCredential struct {
	APIVersion string `json:"apiVersion"`
	Spec       struct {
		Cluster struct {
			Config map[string]interface{} `json:"config"`
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
	client := flyutil.ClientFromContext(ctx)

	execInfo := os.Getenv(execInfoEnv)
	if execInfo == "" {
		return errors.New("KUBERNETES_EXEC_INFO env var is unset or empty")
	}

	var execCredential PartialExecCredential
	err := json.Unmarshal([]byte(execInfo), &execCredential)
	if err != nil {
		return fmt.Errorf("failed to decode ExecCredential: %w", err)
	}

	resp := response{
		APIVersion: execCredential.APIVersion,
		Kind:       "ExecCredential",
	}

	orgSlug, ok := execCredential.Spec.Cluster.Config["org"].(string)
	if !ok {
		return errors.New("org not found in cluster config")
	}

	org, err := orgs.OrgFromSlug(ctx, orgSlug)
	if err != nil {
		return fmt.Errorf("could not find org id for org %s", orgSlug)
	}

	configDir, err := helpers.GetConfigDirectory()
	if err != nil {
		return fmt.Errorf("Error accessing home directory: %w", err)
	}

	fksConfigDir := filepath.Join(configDir, "fks", orgSlug)
	configPath := filepath.Join(fksConfigDir, "config.yml")

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	var token string
	var expiry int64

	err = v.ReadInConfig()
	if err != nil {
		// Generate a new token
		if !helpers.DirectoryExists(fksConfigDir) {
			if err := os.MkdirAll(fksConfigDir, 0o700); err != nil {
				return err
			}
		}

		token, expiry, err = makeOrgToken(ctx, client, org.ID)
		if err != nil {
			return err
		}
	} else {
		// Use existing token
		token = v.GetString("auth.token")
		expiry = int64(v.GetInt("auth.expiration"))
		if time.Now().Unix() > expiry {
			// expired, generate a new token
			token, expiry, err = makeOrgToken(ctx, client, org.ID)
			if err != nil {
				return err
			}
		}
	}

	v.Set("auth.token", token)
	v.Set("auth.expiration", expiry)
	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("could not write fks config file: %w", err)
	}

	resp.Status.Token = token
	resp.Status.Expiration = time.Unix(expiry, 0).Format(time.RFC3339Nano)

	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(resp); err != nil {
		return err
	}

	fmt.Println(buffer.String())
	return nil
}

func makeOrgToken(ctx context.Context, apiClient flyutil.Client, orgID string) (string, int64, error) {
	options := gql.LimitedAccessTokenOptions{}
	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient(),
		"FKS org deploy token",
		orgID,
		"deploy_organization",
		&options,
		(time.Hour).String(),
	)
	if err != nil {
		return "", 0, fmt.Errorf("failed creating deploy token: %w", err)
	}

	token := resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
	token = strings.TrimPrefix(token, tokenPrefix)
	expiry := time.Now().UTC().Add(time.Hour).Unix()
	return token, expiry, nil
}
