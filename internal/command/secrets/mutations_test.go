package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Khan/genqlient/graphql"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
	"gopkg.in/yaml.v3"
)

func mutationContext(t *testing.T, cmd *cobra.Command, args ...string) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("FLY_CONFIG_DIR", dir)
	previous := viper.Get(flyctl.ConfigAppSecretsMinvers)
	t.Cleanup(func() { viper.Set(flyctl.ConfigAppSecretsMinvers, previous) })
	viper.Set(flyctl.ConfigAppSecretsMinvers, config.AppSecretsMinvers{"selected-app": 7, "unrelated": 9})
	require.NoError(t, config.SetAppSecretsMinvers(filepath.Join(dir, config.FileName), config.AppSecretsMinvers{"selected-app": 7, "unrelated": 9}))
	require.NoError(t, cmd.ParseFlags(args))
	ios, _, out, errOut := iostreams.Test()
	ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
	ctx = appconfig.WithName(ctx, "selected-app")
	ctx = state.WithConfigDirectory(ctx, dir)

	return ctx, out, errOut
}

func savedSecretVersions(t *testing.T, ctx context.Context) config.AppSecretsMinvers {
	t.Helper()
	data, err := os.ReadFile(state.ConfigFile(ctx))
	require.NoError(t, err)
	var saved struct {
		Versions config.AppSecretsMinvers `yaml:"app_secrets_minvers"`
	}
	require.NoError(t, yaml.Unmarshal(data, &saved))

	return saved.Versions
}

func TestSecretMutationsStageWithoutDeploying(t *testing.T) {
	for _, operation := range []string{"set", "unset", "import"} {
		t.Run(operation, func(t *testing.T) {
			var cmd *cobra.Command
			var run func(context.Context) error
			args := []string{"--stage"}
			switch operation {
			case "set":
				cmd, run = newSet(), runSet
				args = append(args, "PASSWORD=private-value", "EMPTY=")
			case "unset":
				cmd, run = newUnset(), runUnset
				args = append(args, "PASSWORD", "OLD_KEY")
			case "import":
				cmd, run = newImport(), runImport
				in, err := os.CreateTemp(t.TempDir(), "stdin")
				require.NoError(t, err)
				t.Cleanup(func() { in.Close() })
				_, err = in.WriteString("PASSWORD=private-value\nEMPTY=\n")
				require.NoError(t, err)
				_, err = in.Seek(0, 0)
				require.NoError(t, err)
				old := os.Stdin
				os.Stdin = in
				t.Cleanup(func() { os.Stdin = old })
			}
			ctx, out, errOut := mutationContext(t, cmd, args...)
			updates, deployments := 0, 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
					require.Equal(t, "selected-app", name)
					return &flaps.App{Name: name, Status: "deployed"}, nil
				},
				UpdateAppSecretsFunc: func(_ context.Context, name string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
					updates++
					require.Equal(t, "selected-app", name)
					if operation == "unset" {
						require.Equal(t, map[string]*string{"PASSWORD": nil, "OLD_KEY": nil}, values)
					} else {
						require.Equal(t, map[string]*string{"PASSWORD": fly.Pointer("private-value"), "EMPTY": fly.Pointer("")}, values)
					}

					return &fly.UpdateAppSecretsResp{Version: 42}, nil
				},
				ListFlyAppsMachinesFunc: func(context.Context, string) ([]*fly.Machine, *fly.Machine, error) {
					deployments++
					return nil, nil, nil
				},
			})
			require.NoError(t, run(ctx))
			require.Equal(t, 1, updates)
			require.Zero(t, deployments, "staging must not enter the deployment pipeline")
			require.Contains(t, out.String(), "Secrets have been staged")
			require.NotContains(t, out.String()+errOut.String(), "private-value")
			require.Equal(t, config.AppSecretsMinvers{"selected-app": 42, "unrelated": 9}, savedSecretVersions(t, ctx))
		})
	}
}

func TestSecretMutationUpdateFailurePreventsDeployment(t *testing.T) {
	denied := errors.New("permission denied")
	for _, unset := range []bool{false, true} {
		t.Run(map[bool]string{false: "set", true: "unset"}[unset], func(t *testing.T) {
			ctx, out, errOut := mutationContext(t, newSet())
			deployments := 0
			client := &mock.FlapsClient{
				UpdateAppSecretsFunc: func(context.Context, string, map[string]*string) (*fly.UpdateAppSecretsResp, error) {
					return nil, denied
				},
				GetAppFunc: func(context.Context, string) (*flaps.App, error) { deployments++; return nil, denied },
			}
			ctx = flapsutil.NewContextWithClient(ctx, client)
			var err error
			if unset {
				err = UnsetSecretsAndDeploy(ctx, client, &flaps.App{Name: "selected-app"}, []string{"PASSWORD"}, DeploymentArgs{})
			} else {
				err = SetSecretsAndDeploy(ctx, client, &flaps.App{Name: "selected-app"}, map[string]string{"PASSWORD": "private-value"}, DeploymentArgs{})
			}
			require.ErrorIs(t, err, denied)
			require.Zero(t, deployments)
			require.Empty(t, out.String())
			require.NotContains(t, err.Error()+errOut.String(), "private-value")
			require.Equal(t, config.AppSecretsMinvers{"selected-app": 7, "unrelated": 9}, savedSecretVersions(t, ctx))
		})
	}
}

func TestSecretMutationReportsDeploymentFailureAndUsesRemoteConfig(t *testing.T) {
	deployErr := errors.New("machine service unavailable")
	for _, unset := range []bool{false, true} {
		t.Run(map[bool]string{false: "set", true: "unset"}[unset], func(t *testing.T) {
			ctx, out, errOut := mutationContext(t, newSet())
			local := &appconfig.Config{AppName: "local-app", Env: map[string]string{"LOCAL_ONLY": "do-not-deploy"}}
			ctx = appconfig.WithConfig(ctx, local)
			ctx = flyutil.NewContextWithClient(ctx, &mock.Client{GenqClientFunc: func() graphql.Client {
				return mock.GraphQLClient(func(_ context.Context, req *graphql.Request, resp *graphql.Response) error {
					require.Equal(t, "FlyctlConfigCurrentRelease", req.OpName)
					variables, err := json.Marshal(req.Variables)
					require.NoError(t, err)
					require.JSONEq(t, `{"appName":"selected-app"}`, string(variables))

					return json.Unmarshal([]byte(`{"app":{"currentReleaseUnprocessed":{"configDefinition":{"env":{"REMOTE_ONLY":"keep-me"},"processes":{"app":"server"}}}}}`), resp.Data)
				})
			}})
			listCalls := 0
			client := &mock.FlapsClient{
				UpdateAppSecretsFunc: func(context.Context, string, map[string]*string) (*fly.UpdateAppSecretsResp, error) {
					return &fly.UpdateAppSecretsResp{Version: 42}, nil
				},
				GetAppFunc: func(context.Context, string) (*flaps.App, error) {
					return &flaps.App{Name: "selected-app", Status: "deployed"}, nil
				},
				ListFlyAppsMachinesFunc: func(ctx context.Context, name string) ([]*fly.Machine, *fly.Machine, error) {
					listCalls++
					require.Equal(t, "selected-app", name)
					if listCalls == 1 {
						return []*fly.Machine{{ID: "one"}}, nil, nil
					}
					cfg := appconfig.ConfigFromContext(ctx)
					require.Equal(t, "selected-app", cfg.AppName)
					require.Equal(t, map[string]string{"REMOTE_ONLY": "keep-me"}, cfg.Env)
					require.Equal(t, map[string]string{"app": "server"}, cfg.Processes)

					return nil, nil, deployErr
				},
			}
			ctx = flapsutil.NewContextWithClient(ctx, client)
			var err error
			if unset {
				err = UnsetSecretsAndDeploy(ctx, client, &flaps.App{Name: "selected-app"}, []string{"PASSWORD"}, DeploymentArgs{})
			} else {
				err = SetSecretsAndDeploy(ctx, client, &flaps.App{Name: "selected-app"}, map[string]string{"PASSWORD": "private-value"}, DeploymentArgs{})
			}
			require.ErrorIs(t, err, deployErr)
			require.ErrorContains(t, err, "deployment failed")
			require.Equal(t, 2, listCalls)
			require.Equal(t, config.AppSecretsMinvers{"selected-app": 42, "unrelated": 9}, savedSecretVersions(t, ctx))
			require.Equal(t, map[string]string{"LOCAL_ONLY": "do-not-deploy"}, local.Env)
			require.NotContains(t, err.Error()+out.String()+errOut.String(), "private-value")
		})
	}
}

func TestDeploySecretsOnUndeployedAppStagesForFirstDeploy(t *testing.T) {
	ctx, out, _ := mutationContext(t, newSet())
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetAppFunc:              func(context.Context, string) (*flaps.App, error) { return &flaps.App{Name: "selected-app"}, nil },
		ListFlyAppsMachinesFunc: func(context.Context, string) ([]*fly.Machine, *fly.Machine, error) { return nil, nil, nil },
	})
	require.NoError(t, DeploySecrets(ctx, "selected-app", DeploymentArgs{}))
	require.Equal(t, "Secrets are staged for the first deployment\n", out.String())
}

func TestRunSetInvalidInputDoesNotUpdateSecrets(t *testing.T) {
	ctx, out, _ := mutationContext(t, newSet(), "MISSING_EQUALS")
	calls := 0
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		GetAppFunc: func(context.Context, string) (*flaps.App, error) { return &flaps.App{Name: "selected-app"}, nil },
		UpdateAppSecretsFunc: func(context.Context, string, map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			calls++
			return nil, nil
		},
	})
	require.ErrorContains(t, runSet(ctx), "could not parse secrets")
	require.Zero(t, calls)
	require.Empty(t, out.String())
}

func TestSecretMutationReportsRemoteSuccessWhenVersionCannotBeSaved(t *testing.T) {
	ctx, out, errOut := mutationContext(t, newSet())
	// A directory in place of config.yml fails consistently even when tests
	// run as root, unlike a file made read-only with chmod.
	require.NoError(t, os.Remove(state.ConfigFile(ctx)))
	require.NoError(t, os.Mkdir(state.ConfigFile(ctx), 0700))
	updates, deployments := 0, 0
	client := &mock.FlapsClient{
		UpdateAppSecretsFunc: func(context.Context, string, map[string]*string) (*fly.UpdateAppSecretsResp, error) {
			updates++
			return &fly.UpdateAppSecretsResp{Version: 42}, nil
		},
		GetAppFunc: func(context.Context, string) (*flaps.App, error) {
			deployments++
			return nil, errors.New("unexpected deployment")
		},
	}
	ctx = flapsutil.NewContextWithClient(ctx, client)
	err := SetSecretsAndDeploy(ctx, client, &flaps.App{Name: "selected-app"}, map[string]string{"PASSWORD": "private-value"}, DeploymentArgs{})
	require.ErrorContains(t, err, "secrets were updated remotely")
	require.Equal(t, 1, updates)
	require.Zero(t, deployments)
	require.NotContains(t, err.Error()+out.String()+errOut.String(), "private-value")
}
