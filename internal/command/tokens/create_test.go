package tokens

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	flytokens "github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

func permissionToken(t *testing.T) string {
	t.Helper()
	m, err := macaroon.New([]byte("test-token"), flyio.LocationPermission, macaroon.NewSigningKey())
	require.NoError(t, err)
	require.NoError(t, m.Add(&flyio.Organization{ID: 42, Mask: resset.ActionAll}))
	b, err := m.Encode()
	require.NoError(t, err)

	return macaroon.ToAuthorizationHeader(b)
}

func decodePermission(t *testing.T, token string) []*macaroon.Macaroon {
	t.Helper()
	encoded, err := macaroon.Parse(token)
	require.NoError(t, err)
	perms, _, _, _, err := macaroon.FindPermissionAndDischargeTokens(encoded, flyio.LocationPermission)
	require.NoError(t, err)

	return perms
}

func TestCreateScopedTokens(t *testing.T) {
	for _, tc := range []struct {
		name       string
		newCommand func() *cobra.Command
		run        func(context.Context) error
		appScoped  bool
	}{
		{"deploy", newDeploy, runDeploy, true},
		{"org", newOrg, runOrg, false},
		{"readonly", newOrgRead, runOrgRead, false},
		{"ssh", newSSH, runSSH, true},
		{"machine-exec", newMachineExec, runMachineExec, true},
		{"wireguard", newWireGuard, runWireGuard, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FLY_ORG", "unrelated-env-org")
			cmd := tc.newCommand()
			args := []string{"--name", "CI job", "--expiry", "2h"}
			if !tc.appScoped {
				args = append(args, "--org", "selected-org")
			}
			if tc.name == "machine-exec" {
				args = append(args, "--command", "echo 'hello world'", "--command-prefix", "ls /srv", "ps", "aux")
			}
			require.NoError(t, cmd.ParseFlags(args))
			ios, _, out, errOut := iostreams.Test()
			ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
			ctx = config.NewContext(ctx, &config.Config{JSONOutput: true})
			ctx = appconfig.WithName(ctx, "selected-app")
			appCalls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetAppFunc: func(_ context.Context, name string) (*flaps.App, error) {
					appCalls++
					require.Equal(t, "selected-app", name)
					return &flaps.App{Name: name, Organization: flaps.AppOrganizationInfo{Slug: "selected-org"}}, nil
				},
			})
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
					require.Equal(t, "selected-org", slug)
					return &uiex.Organization{ID: "org-42", Slug: slug}, nil
				},
			})
			issued := permissionToken(t)
			requests := 0
			ctx = flyutil.NewContextWithClient(ctx, &mock.Client{GenqClientFunc: func() graphql.Client {
				return mock.GraphQLClient(func(_ context.Context, req *graphql.Request, resp *graphql.Response) error {
					requests++
					require.Equal(t, "CreateLimitedAccessToken", req.OpName)
					vars, err := json.Marshal(req.Variables)
					require.NoError(t, err)
					var got map[string]any
					require.NoError(t, json.Unmarshal(vars, &got))
					expected := map[string]any{"name": "CI job", "organizationId": "org-42", "expiry": "2h0m0s", "profile": "deploy_organization", "profileParams": map[string]any{}}
					if tc.appScoped {
						expected["profile"] = "deploy"
						expected["profileParams"] = map[string]any{"app_id": "selected-app"}
					}
					require.Equal(t, expected, got)
					data, err := json.Marshal(map[string]any{"createLimitedAccessToken": map[string]any{"limitedAccessToken": map[string]string{"tokenHeader": issued}}})
					require.NoError(t, err)

					return json.Unmarshal(data, resp.Data)
				})
			}})
			require.NoError(t, tc.run(ctx))
			require.Equal(t, 1, requests)
			if tc.appScoped {
				require.Equal(t, 1, appCalls)
			} else {
				require.Zero(t, appCalls)
			}
			require.Empty(t, errOut.String())
			var result map[string]string
			require.NoError(t, json.Unmarshal(out.Bytes(), &result))
			require.Len(t, result, 1)
			perms := decodePermission(t, result["token"])
			switch tc.name {
			case "readonly":
				require.Len(t, perms, 1)
				orgs := macaroon.GetCaveats[*flyio.Organization](&perms[0].UnsafeCaveats)
				require.Contains(t, orgs, &flyio.Organization{ID: 42, Mask: resset.ActionRead})
			case "ssh":
				require.Len(t, perms, 2)
				read := macaroon.GetCaveats[*resset.Action](&perms[0].UnsafeCaveats)
				require.Len(t, read, 1)
				require.Equal(t, resset.ActionRead, *read[0])
				mutations := macaroon.GetCaveats[*flyio.Mutations](&perms[1].UnsafeCaveats)
				require.Len(t, mutations, 1)
				require.ElementsMatch(t, []string{"issueCertificate", "addWireGuardPeer"}, mutations[0].Mutations)
			case "machine-exec":
				require.Len(t, perms, 1)
				cavs := macaroon.GetCaveats[*resset.IfPresent](&perms[0].UnsafeCaveats)
				require.Len(t, cavs, 1)
				require.Equal(t, resset.ActionRead, cavs[0].Else)
				commands := macaroon.GetCaveats[*flyio.Commands](cavs[0].Ifs)
				require.Len(t, commands, 1)
				require.Equal(t, flyio.Commands{{Args: []string{"ps", "aux"}, Exact: true}, {Args: []string{"echo", "hello world"}, Exact: true}, {Args: []string{"ls", "/srv"}, Exact: false}}, *commands[0])
			case "wireguard":
				require.Len(t, perms, 1)
				cavs := macaroon.GetCaveats[*resset.IfPresent](&perms[0].UnsafeCaveats)
				require.Len(t, cavs, 1)
				require.Equal(t, resset.ActionRead, cavs[0].Else)
				apps := macaroon.GetCaveats[*flyio.Apps](cavs[0].Ifs)
				require.Len(t, apps, 1)
				require.Empty(t, apps[0].Apps)
				features := macaroon.GetCaveats[*flyio.FeatureSet](cavs[0].Ifs)
				require.Len(t, features, 1)
				require.Equal(t, resset.ResourceSet[string, resset.Action]{flyio.FeatureWireGuard: resset.ActionAll}, features[0].Features)
			default:
				require.Equal(t, issued, result["token"])
			}
		})
	}
}

func TestCreateDeployTokenFailureDoesNotPrintToken(t *testing.T) {
	denied := errors.New("permission denied")
	cmd := newDeploy()
	require.NoError(t, cmd.ParseFlags([]string{"--expiry", "0"}))
	ios, _, out, errOut := iostreams.Test()
	ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
	ctx = appconfig.WithName(ctx, "selected-app")
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{GetAppFunc: func(context.Context, string) (*flaps.App, error) {
		return &flaps.App{Name: "selected-app", Organization: flaps.AppOrganizationInfo{Slug: "owner"}}, nil
	}})
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{GetOrganizationFunc: func(context.Context, string) (*uiex.Organization, error) {
		return &uiex.Organization{ID: "org-42"}, nil
	}})
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{GenqClientFunc: func() graphql.Client {
		return mock.GraphQLClient(func(_ context.Context, req *graphql.Request, _ *graphql.Response) error {
			data, err := json.Marshal(req.Variables)
			require.NoError(t, err)
			require.Contains(t, string(data), `"expiry":""`)
			return denied
		})
	}})
	require.ErrorIs(t, runDeploy(ctx), denied)
	require.Empty(t, out.String()+errOut.String())
}

func TestReadonlyFromExistingAddsExpiryWithoutIssuingToken(t *testing.T) {
	issued := permissionToken(t)
	cmd := newOrgRead()
	require.NoError(t, cmd.ParseFlags([]string{"--from-existing", "--expiry", "1h"}))
	ios, _, out, _ := iostreams.Test()
	ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
	ctx = config.NewContext(ctx, &config.Config{Tokens: flytokens.Parse(issued)})
	before := time.Now().Unix()
	require.NoError(t, runOrgRead(ctx))
	perms := decodePermission(t, strings.TrimSpace(out.String()))
	require.Len(t, perms, 1)
	orgs := macaroon.GetCaveats[*flyio.Organization](&perms[0].UnsafeCaveats)
	require.Contains(t, orgs, &flyio.Organization{ID: 42, Mask: resset.ActionRead})
	windows := macaroon.GetCaveats[*macaroon.ValidityWindow](&perms[0].UnsafeCaveats)
	require.Len(t, windows, 1)
	require.GreaterOrEqual(t, windows[0].NotBefore, before)
	require.LessOrEqual(t, windows[0].NotAfter, time.Now().Add(time.Hour).Unix())
	require.InDelta(t, int64(time.Hour/time.Second), windows[0].NotAfter-windows[0].NotBefore, 1)
}
