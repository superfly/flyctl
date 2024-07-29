package tokens

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newCreate() *cobra.Command {
	const (
		short = "Create Fly.io API tokens"
		long  = "Create Fly.io API tokens"
		usage = "create"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newDeploy(),
		newMachineExec(),
		newOrg(),
		newOrgRead(),
		newLiteFSCloud(),
		newSSH(),
	)

	return cmd
}

func newOrg() *cobra.Command {
	const (
		short = "Create org deploy tokens"
		long  = "Create an API token limited to managing a single org and its resources. Tokens are valid for 20 years by default. We recommend using a shorter expiry if practical."
		usage = "org"
	)

	cmd := command.New(usage, short, long, runOrg,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour * 24 * 365 * 20,
		},
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Token name",
			Default:     "Org deploy token",
		},
		flag.Org(),
	)

	return cmd
}

func newSSH() *cobra.Command {
	const (
		short = "Create token for SSH'ing to a single app"
		long  = "Create token for SSH'ing to a single app. To be able to SSH to an app, this token is also allowed to connect to the org's wireguard network."
		usage = "ssh"
	)

	cmd := command.New(usage, short, long, runSSH,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Token name",
			Default:     "flyctl ssh token",
		},
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour * 24 * 365 * 20,
		},
	)

	return cmd
}

func newOrgRead() *cobra.Command {
	const (
		short = "Create read-only org tokens"
		long  = "Create an API token limited to reading a single org and its resources. Tokens are valid for 20 years by default. We recommend using a shorter expiry if practical."
		usage = "readonly"
	)

	cmd := command.New(usage, short, long, runOrgRead,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Bool{
			Name:        "from-existing",
			Description: "Use an existing token as the basis for the read-only token",
		},
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour * 24 * 365 * 20,
		},
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Token name",
			Default:     "Read-only org token",
		},
	)

	return cmd
}

func newDeploy() *cobra.Command {
	const (
		short = "Create deploy tokens"
		long  = "Create an API token limited to managing a single app and its resources. Also available as TOKENS DEPLOY. Tokens are valid for 20 years by default. We recommend using a shorter expiry if practical."
		usage = "deploy"
	)

	cmd := command.New(usage, short, long, runDeploy,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Token name",
			Default:     "flyctl deploy token",
		},
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour * 24 * 365 * 20,
		},
	)

	return cmd
}

func newLiteFSCloud() *cobra.Command {
	const (
		short = "Create LiteFS Cloud tokens"
		long  = "Create an API token limited to a single LiteFS Cloud cluster."
		usage = "litefs-cloud"
	)

	cmd := command.New(usage, short, long, runLiteFSCloud,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Org(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Token name",
			Default:     "LiteFS Cloud token",
		},
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour * 24 * 365 * 20,
		},
		flag.String{
			Name:        "cluster",
			Shorthand:   "c",
			Description: "Cluster name",
		},
	)

	return cmd
}

func newMachineExec() *cobra.Command {
	const (
		short = "Create a machine exec token"
		long  = "Create an API token that can execute a restricted set of commands on a machine. Commands can be specified on the command line or with the command and command-prefix flags. If no command is provided, all commands are allowed. Tokens are valid for 20 years by default. We recommend using a shorter expiry if practical."
		usage = "machine-exec [command...]"
	)

	cmd := command.New(usage, short, long, runMachineExec,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Token name",
			Default:     "flyctl machine-exec token",
		},
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour * 24 * 365 * 20,
		},
		flag.StringSlice{
			Name:        "command",
			Shorthand:   "C",
			Description: "An allowed command with arguments. This command must match exactly",
		},
		flag.StringSlice{
			Name:        "command-prefix",
			Shorthand:   "p",
			Description: "An allowed command with arguments. This command must match the prefix of a command",
		},
	)

	return cmd
}

func makeToken(ctx context.Context, apiClient flyutil.Client, orgID string, expiry string, profile string, options *gql.LimitedAccessTokenOptions) (*gql.CreateLimitedAccessTokenResponse, error) {
	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient(),
		flag.GetString(ctx, "name"),
		orgID,
		profile,
		options,
		expiry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating token: %w", err)
	}
	return resp, nil
}

func runOrg(ctx context.Context) error {
	var token string
	apiClient := flyutil.ClientFromContext(ctx)

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	org, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving org %w", err)
	}

	resp, err := makeToken(ctx, apiClient, org.ID, expiry, "deploy_organization", &gql.LimitedAccessTokenOptions{})
	if err != nil {
		return err
	}

	token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}

func runSSH(ctx context.Context) error {
	var token string
	apiClient := flyutil.ClientFromContext(ctx)

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	// start with app deploy token and then pare it down.
	resp, err := makeToken(ctx, apiClient, app.Organization.ID, expiry, "deploy", &gql.LimitedAccessTokenOptions{
		"app_id": app.ID,
	})
	if err != nil {
		return err
	}

	token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
	macTok, disToks, err := flyio.ParsePermissionAndDischargeTokens(token)
	if err != nil {
		return fmt.Errorf("failed parsing token from API: %w", err)
	}

	// FindPermissionAndDischargeTokens returned parsed tokens, but we want to
	// make two copies of the token and there's no API for doing a deep copy of
	// a Macaroon.
	orgAppReadMac, err := macaroon.Decode(macTok)
	if err != nil {
		return fmt.Errorf("failed decoding tokens from API: %w", err)
	}

	if err := orgAppReadMac.Add(ptr(resset.ActionRead)); err != nil {
		return fmt.Errorf("failed to attenuate org-app-read token: %w", err)
	}

	orgAppReadTok, err := orgAppReadMac.Encode()
	if err != nil {
		return fmt.Errorf("failed encoding org-app-read token: %w", err)
	}

	mutationMac, err := macaroon.Decode(macTok)
	if err != nil {
		return fmt.Errorf("failed decoding tokens from API: %w", err)
	}

	if err := mutationMac.Add(&flyio.Mutations{Mutations: []string{"issueCertificate", "addWireGuardPeer"}}); err != nil {
		return fmt.Errorf("failed to attenuate mutation token: %w", err)
	}

	mutationTok, err := mutationMac.Encode()
	if err != nil {
		return fmt.Errorf("failed encoding mutation token: %w", err)
	}

	token = macaroon.ToAuthorizationHeader(append([][]byte{orgAppReadTok, mutationTok}, disToks...)...)

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}

func runOrgRead(ctx context.Context) error {
	var (
		token          string
		apiClient      = flyutil.ClientFromContext(ctx)
		expiry         = ""
		expiryDuration = flag.GetDuration(ctx, "expiry")
		perm           []byte
		diss           [][]byte
	)

	if expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	if !flag.GetBool(ctx, "from-existing") {
		org, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
		if err != nil {
			return fmt.Errorf("failed retrieving org %w", err)
		}

		resp, err := makeToken(ctx, apiClient, org.ID, expiry, "deploy_organization", &gql.LimitedAccessTokenOptions{})
		if err != nil {
			return err
		}

		token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

		perm, diss, err = macaroon.ParsePermissionAndDischargeTokens(token, flyio.LocationPermission)
		if err != nil {
			return err
		}
	} else { /* see also below: we add expiry explicitly if this branch is taken */
		toks, err := getTokens(ctx)
		if err != nil {
			return err
		}

		var perms [][]byte

		_, perms, _, diss, err = macaroon.FindPermissionAndDischargeTokens(toks, flyio.LocationPermission)
		if err != nil {
			return err
		}

		if len(perms) > 1 {
			return fmt.Errorf("the currently set token string has more than one permission token in it, can't proceed")
		}

		perm = perms[0]
	}

	mac, err := macaroon.Decode(perm)
	if err != nil {
		return err
	}

	// attenuate to read-only
	var orgID *uint64
	for _, cav := range macaroon.GetCaveats[*flyio.Organization](&mac.UnsafeCaveats) {
		if orgID != nil {
			return errors.New("multiple org caveats")
		}
		orgID = &cav.ID
	}
	if orgID == nil {
		return errors.New("no org caveats")
	}
	if err := mac.Add(&flyio.Organization{ID: *orgID, Mask: resset.ActionRead}); err != nil {
		return err
	}

	if expiryDuration != 0 && flag.GetBool(ctx, "from-existing") {
		if err := mac.Add(&macaroon.ValidityWindow{
			NotBefore: time.Now().Unix(),
			NotAfter:  time.Now().Add(expiryDuration).Unix(),
		}); err != nil {
			return err
		}
	}

	if perm, err = mac.Encode(); err != nil {
		return err
	}

	token = macaroon.ToAuthorizationHeader(append([][]byte{perm}, diss...)...)

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}

func runDeploy(ctx context.Context) (err error) {
	var token string
	apiClient := flyutil.ClientFromContext(ctx)

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	resp, err := makeToken(ctx, apiClient, app.Organization.ID, expiry, "deploy", &gql.LimitedAccessTokenOptions{
		"app_id": app.ID,
	})
	if err != nil {
		return err
	}

	token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}

func runMachineExec(ctx context.Context) error {
	var token string
	apiClient := flyutil.ClientFromContext(ctx)

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	resp, err := makeToken(ctx, apiClient, app.Organization.ID, expiry, "deploy", &gql.LimitedAccessTokenOptions{
		"app_id": app.ID,
	})
	if err != nil {
		return err
	}

	token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
	cmdCav, err := getCommandCaveat(ctx)
	if err != nil {
		return err
	}

	token, err = attenuate(token, cmdCav)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}

func attenuate(token string, cavs ...macaroon.Caveat) (string, error) {
	var atoken string
	macTok, disToks, err := flyio.ParsePermissionAndDischargeTokens(token)
	if err != nil {
		return atoken, fmt.Errorf("failed parsing token from API: %w", err)
	}

	mac, err := macaroon.Decode(macTok)
	if err != nil {
		return atoken, err
	}

	if err := mac.Add(cavs...); err != nil {
		return atoken, err
	}

	perm, err := mac.Encode()
	if err != nil {
		return atoken, err
	}

	atoken = macaroon.ToAuthorizationHeader(append([][]byte{perm}, disToks...)...)
	return atoken, nil
}

func getCommandCaveat(ctx context.Context) (macaroon.Caveat, error) {
	commands := flyio.Commands{}
	if args := flag.Args(ctx); len(args) > 0 {
		cav := flyio.Command{
			Args:  args,
			Exact: true,
		}
		commands = append(commands, cav)
	}

	for _, cmd := range flag.GetStringSlice(ctx, "command") {
		args, err := shlex.Split(cmd)
		if err != nil {
			return nil, fmt.Errorf("cant parse `%s`: %w", cmd, err)
		}

		cav := flyio.Command{
			Args:  args,
			Exact: true,
		}
		commands = append(commands, cav)
	}

	for _, cmd := range flag.GetStringSlice(ctx, "command-prefix") {
		args, err := shlex.Split(cmd)
		if err != nil {
			return nil, fmt.Errorf("cant parse `%s`: %w", cmd, err)
		}

		cav := flyio.Command{
			Args:  args,
			Exact: false,
		}
		commands = append(commands, cav)
	}

	if len(commands) == 0 {
		cav := flyio.Command{
			Args:  []string{},
			Exact: false,
		}
		commands = append(commands, cav)
	}

	cav := &resset.IfPresent{
		Ifs:  macaroon.NewCaveatSet(&commands),
		Else: resset.ActionRead,
	}

	return cav, nil
}

func runLiteFSCloud(ctx context.Context) (err error) {
	var token string
	apiClient := flyutil.ClientFromContext(ctx)

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	cluster := flag.GetString(ctx, "cluster")
	if cluster == "" {
		return fmt.Errorf("cluster name is not provided")
	}

	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving org %w", err)
	}

	resp, err := makeToken(ctx, apiClient, org.ID, expiry, "litefs_cloud", &gql.LimitedAccessTokenOptions{
		"cluster": cluster,
	})
	if err != nil {
		return err
	}

	token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}

func ptr[T any](t T) *T {
	return &t
}
