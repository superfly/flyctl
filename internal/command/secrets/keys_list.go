package secrets

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
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

func compareSecrets(a, b fly.SecretKey) int {
	aver, aprefix, err1 := SplitLabelKeyver(a.Name)
	if err1 != nil {
		return -1
	}
	bver, bprefix, err2 := SplitLabelKeyver(b.Name)
	if err2 != nil {
		return 1
	}

	diff := strings.Compare(aprefix, bprefix)
	if diff != 0 {
		return diff
	}

	diff = CompareKeyver(aver, bver)
	return diff
}

type jsonSecret struct {
	Label   string `json:"label"`
	Name    string `json:"name"`
	Version string `json:"version"`
	SemType string `json:"type"`
	Type    string `json:"secret_type"`
}

func runKeysList(ctx context.Context) (err error) {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	appName := appconfig.NameFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	secrets, err := flapsClient.ListSecretKeys(ctx, appName, nil)
	if err != nil {
		return err
	}

	var rows [][]string
	var jsecrets []jsonSecret
	slices.SortFunc(secrets, compareSecrets)
	for _, secret := range secrets {
		semType, err := SecretTypeToSemanticType(secret.Type)
		if err != nil {
			continue
		}

		ver, prefix, err := SplitLabelKeyver(secret.Name)
		if err != nil {
			continue
		}
		jsecret := jsonSecret{
			Label:   secret.Name,
			Name:    prefix,
			Version: ver.String(),
			SemType: string(semType),
			Type:    secretTypeToString(secret.Type),
		}

		jsecrets = append(jsecrets, jsecret)
		rows = append(rows, []string{
			jsecret.Label,
			jsecret.Name,
			jsecret.Version,
			fmt.Sprintf("%s (%s)", jsecret.SemType, jsecret.Type),
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
