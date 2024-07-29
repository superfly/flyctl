package postgres

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func newRenewSSHCerts() *cobra.Command {
	const (
		short = "Renews the SSH certificates for the Postgres cluster."
		long  = "Renews the SSH certificates for the Postgres cluster. This is useful when the certificates have expired or need to be rotated."
		usage = "renew-certs"
	)

	cmd := command.New(usage, short, long, runRefreshSSHCerts,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Int{
			Name:        "valid-days",
			Description: "The number of days the certificate should be valid for.",
			Default:     36525,
		},
	)

	return cmd
}

func runRefreshSSHCerts(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		client  = flyutil.ClientFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	return refreshSSHCerts(ctx, app)
}

func refreshSSHCerts(ctx context.Context, app *fly.AppCompact) error {
	var (
		io        = iostreams.FromContext(ctx)
		client    = flyutil.ClientFromContext(ctx)
		colorize  = io.ColorScheme()
		validDays = flag.GetInt(ctx, "valid-days")
	)

	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc()
	if err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return fmt.Errorf("failed to resolve leader: %w", err)
	}

	if !IsFlex(leader) {
		return fmt.Errorf("app %s is not a flex cluster", app.Name)
	}

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("failed to generate ssh key: %w", err)
	}

	validHours := validDays * 24
	cert, err := client.IssueSSHCertificate(ctx, app.Organization, []string{"root", "fly", "postgres"}, []string{app.Name}, &validHours, pub)
	if err != nil {
		return fmt.Errorf("failed to issue ssh certificate: %w", err)
	}

	pemkey := ssh.MarshalED25519PrivateKey(priv, "postgres inter-machine ssh")

	secrets := map[string]string{
		"SSH_KEY":  string(pemkey),
		"SSH_CERT": cert.Certificate,
	}

	_, err = client.SetSecrets(ctx, app.Name, secrets)
	if err != nil {
		return fmt.Errorf("failed to set ssh secrets: %w", err)
	}

	command := fmt.Sprintf("fly deploy --app %s --image %s", app.Name, leader.FullImageRef())

	fmt.Fprintf(io.Out, "Your SSH certificate(s) have been renewed are set to expire in %d day(s)\n", validDays)
	fmt.Fprintf(io.Out, "Run %s to apply the changes!\n", colorize.Bold(command))

	return nil
}
