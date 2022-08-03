package secrets

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newImport() (cmd *cobra.Command) {
	const (
		long  = `Set one or more encrypted secrets for an application. Values are read from stdin as NAME=VALUE pairs`
		short = `Set secrets as NAME=VALUE pairs from stdin`
		usage = "import [flags]"
	)

	cmd = command.New(usage, short, long, runImport, command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		sharedFlags,
	)

	return cmd
}

func runImport(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)

	if err != nil {
		return
	}

	secrets := make(map[string]string)

	secretsString, err := ioutil.ReadAll(os.Stdin)

	if err != nil {
		return err
	}

	secretsArray := strings.Split(string(secretsString), "\n")

	parsestate := 0
	parsedkey := ""
	var parsebuffer strings.Builder

	for _, line := range secretsArray {
		switch parsestate {
		case 0:
			if line != "" {
				parts := strings.SplitN(line, "=", 2)
				if strings.HasPrefix(parts[1], "\"\"\"") {
					// Switch to multiline
					parsestate = 1
					parsedkey = parts[0]
					parsebuffer.WriteString(strings.TrimPrefix(parts[1], "\"\"\""))
					parsebuffer.WriteString("\n")
				} else {
					if len(parts) != 2 {
						return fmt.Errorf("Secrets must be provided as NAME=VALUE pairs (%s is invalid)", line)
					}
					key := parts[0]
					value := parts[1]
					secrets[key] = value
				}
			}
		case 1:
			if strings.HasSuffix(line, "\"\"\"") {
				// End of multiline
				parsebuffer.WriteString(strings.TrimSuffix(line, "\"\"\""))
				secrets[parsedkey] = parsebuffer.String()
				parsebuffer.Reset()
				parsestate = 0
				parsedkey = ""
			} else {
				if line != "" {
					parsebuffer.WriteString(line)
				}
				parsebuffer.WriteString("\n")
			}

		}

	}

	if len(secrets) < 1 {
		return errors.New("requires at least one SECRET=VALUE pair")
	}

	release, err := client.SetSecrets(ctx, appName, secrets)

	if err != nil {
		return err
	}

	return deployForSecrets(ctx, app, release)
}
