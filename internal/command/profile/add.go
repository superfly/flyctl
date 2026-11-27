package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/auth/webauth"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	profilelib "github.com/superfly/flyctl/internal/profile"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func newAdd() *cobra.Command {
	const (
		short = "Add a profile and log a Fly.io account into it"
		long  = `Creates a new, empty config directory for the named profile and
logs an account into it, leaving every other profile untouched.

By default this opens a browser to log in. Pass --token to store an existing
API token instead, which is what CI and scripted setups want.

Adding a profile does not switch to it. Pass --use to do both at once.
`
	)

	cmd := command.New("add <name>", short, long, runAdd)

	cmd.Aliases = []string{"create", "new"}
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "token",
			Description: "Store this API token instead of opening a browser to log in",
		},
		flag.Bool{
			Name:        "use",
			Description: "Make the new profile active once it is created",
		},
	)

	return cmd
}

func runAdd(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	cs := io.ColorScheme()

	name := flag.FirstArg(ctx)
	if err := profilelib.ValidateName(name); err != nil {
		return err
	}

	dir, err := profilelib.Create(name)
	if err != nil {
		return err
	}

	// Anything that fails from here leaves a half-built profile behind, which
	// would then resolve to an account-less config directory. Clean it up so
	// the failure is total rather than partial.
	committed := false
	defer func() {
		if !committed {
			_ = profilelib.Remove(name)
		}
	}()

	// Point the login flow at the new profile's directory rather than the one
	// this command resolved to.
	loginCtx := state.WithConfigDirectory(ctx, dir)

	token := flag.GetString(ctx, "token")
	if token == "" {
		if token, err = webauth.RunWebLogin(loginCtx, false); err != nil {
			return err
		}
	}

	if err := webauth.SaveToken(loginCtx, token); err != nil {
		return err
	}

	// SaveToken already greeted the user by email, but it does not hand the
	// address back, so ask once more to cache it for `fly profile list`.
	email, err := currentUserEmail(ctx, token)
	if err != nil {
		return err
	}

	now := time.Now()
	if err := profilelib.WriteMetadata(name, profilelib.Metadata{
		Email:      email,
		CreatedAt:  now,
		VerifiedAt: now,
	}); err != nil {
		return err
	}

	committed = true

	fmt.Fprintf(io.Out, "\ncreated profile %s for %s\n", cs.Bold(name), cs.Bold(email))

	if flag.GetBool(ctx, "use") {
		if err := profilelib.SetActive(name); err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "switched to profile %s\n", cs.Bold(name))

		return nil
	}

	fmt.Fprintf(io.Out, "\nUse it with any of:\n")
	fmt.Fprintf(io.Out, "  fly profile use %s        # switch globally\n", name)
	fmt.Fprintf(io.Out, "  fly profile link %s       # pin this directory tree to it\n", name)
	fmt.Fprintf(io.Out, "  fly --profile %s <cmd>    # just this command\n", name)

	return nil
}

func currentUserEmail(ctx context.Context, token string) (string, error) {
	user, err := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
		AccessToken: token,
	}).GetCurrentUser(ctx)
	if err != nil {
		return "", fmt.Errorf("failed retrieving current user: %w", err)
	}

	return user.Email, nil
}
