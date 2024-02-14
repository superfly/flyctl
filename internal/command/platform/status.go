package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/httptracing"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

const StatusURL = "https://status.fly.io/"

func newStatus() (cmd *cobra.Command) {
	const (
		long = `Show current Fly platform status in a browser or via json with the json flag
`
		short = "Show current platform status with an optional filter for maintenance or incidents in json mode (eg. incidents, maintenance)"
	)

	cmd = command.New("status [kind]", short, long, runStatus)
	cmd.Args = cobra.MaximumNArgs(1)
	flag.Add(cmd, flag.JSONOutput())
	return
}

func runStatus(ctx context.Context) error {
	var (
		cfg               = config.FromContext(ctx)
		getStatusEndpoint string
		getStatusKind     = flag.FirstArg(ctx)
	)

	switch getStatusKind {
	case "brief":
		getStatusEndpoint = "api/v2/status.json"
	case "summary":
		getStatusEndpoint = "api/v2/summary.json"
	case "incidents":
		getStatusEndpoint = "api/v2/incidents/unresolved.json"
	case "maintenance":
		getStatusEndpoint = "api/v2/scheduled-maintenances/active.json"
	case "":
		getStatusEndpoint = "api/v2/status.json"
	default:
		return fmt.Errorf("status subcommand must be empty or of type brief, summary, incidents, maintenance")
	}

	if cfg.JSONOutput {
		httpClient, err := fly.NewHTTPClient(logger.MaybeFromContext(ctx), httptracing.NewTransport(http.DefaultTransport))
		if err != nil {
			return err
		}
		res, err := httpClient.Get(StatusURL + getStatusEndpoint)
		if err != nil {
			return err
		}
		defer res.Body.Close() //skipcq: GO-S2307

		if res.StatusCode != 200 {
			err = fly.ErrorFromResp(res)
			return fmt.Errorf("failed to retrieve status: %w", err)
		}

		var result = map[string]any{}
		if err = json.NewDecoder(res.Body).Decode(&result); err != nil {
			return nil
		}
		out := iostreams.FromContext(ctx).Out
		return render.JSON(out, result)
	}

	w := iostreams.FromContext(ctx).ErrOut
	fmt.Fprintf(w, "opening %s ...\n", StatusURL)

	if err := open.Run(StatusURL); err != nil {
		return fmt.Errorf("failed opening %s: %w", StatusURL, err)
	}

	return nil
}
