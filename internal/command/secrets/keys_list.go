package secrets

import (
	"context"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newKeysList() (cmd *cobra.Command) {
	const (
		long = `List the keys secrets available to the application. It shows each secret's
name and version.`
		short = `List application keys secrets names`
		usage = "list [flags]"
	)

	cmd = command.New(usage, short, long, runKeysList, command.RequireSession, command.RequireAppName)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)

	return cmd
}

func compareSecrets(a, b fly.ListSecret) int {
	aver, aprefix := splitLabelVersion(a.Label)
	bver, bprefix := splitLabelVersion(b.Label)

	diff := strings.Compare(aprefix, bprefix)
	if diff != 0 {
		return diff
	}

	diff = CompareVer(aver, bver)
	return diff
}

type jsonSecret struct {
	Label   string `json:"label"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

func runKeysList(ctx context.Context) (err error) {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	flapsClient, err := getFlapsClient(ctx)
	if err != nil {
		return err
	}

	secrets, err := flapsClient.ListSecrets(ctx)
	if err != nil {
		return err
	}

	var rows [][]string
	var jsecrets []jsonSecret
	slices.SortFunc(secrets, compareSecrets)
	for _, secret := range secrets {
		ver, prefix := splitLabelVersion(secret.Label)
		jsecret := jsonSecret{
			Label:   secret.Label,
			Name:    prefix,
			Version: ver.String(),
			Type:    secretTypeToString(secret.Type),
		}

		jsecrets = append(jsecrets, jsecret)
		rows = append(rows, []string{
			jsecret.Label,
			jsecret.Name,
			jsecret.Version,
			jsecret.Type,
		})
	}

	headers := []string{
		"Label",
		"Name",
		"Version",
		"Type",
	}
	if cfg.JSONOutput {
		return render.JSON(out, jsecrets)
	} else {
		return render.Table(out, "", rows, headers...)
	}
}
