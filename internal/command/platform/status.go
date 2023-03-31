package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newStatus() (cmd *cobra.Command) {
	const (
		long = `Show current Fly platform status in a browser or via json with the json flag
`
		short = "Show current platform status with an optional filter for maintenance or incidents in json mode (eg. incidents, maintenance)"
	)

	cmd = command.New("status [kind](optional)", short, long, runStatus)
	cmd.Args = cobra.MaximumNArgs(1)
	return
}

type Page struct {
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	Url       string    `json:"url,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type Status struct {
	Description string `json:"description,omitempty"`
	Indicator   string `json:"indicator,omitempty"`
}

type incidentUpdates struct {
	Body      string    `json:"body,omitempty"`
	Status    string    `json:"status,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type Incident struct {
	ID              string          `json:"id,omitempty"`
	Name            string          `json:"name,omitempty"`
	Status          string          `json:"status,omitempty"`
	CreatedAt       time.Time       `json:"port,omitempty"`
	Impact          string          `json:"impact,omitempty"`
	MonitoringAt    time.Time       `json:"monitoring_at,omitempty"`
	PageID          string          `json:"page_id,omitempty"`
	ResolvedAt      time.Time       `json:"resolved_at,omitempty"`
	UpdatedAt       time.Time       `json:"updated_at,omitempty"`
	IncidentUpdates incidentUpdates `json:"incident_updates,omitempty"`
}

type StatusPage struct {
	Status Status
	Page   Page
}
type Client struct {
	baseUrl    *url.URL
	httpClient *http.Client
}

func createClient(ctx context.Context, rawUrl string) (*Client, error) {
	cleanedURL, _ := url.Parse(rawUrl)
	logger := logger.MaybeFromContext(ctx)
	httpClient, err := api.NewHTTPClient(logger, http.DefaultTransport)
	if err != nil {
		return nil, fmt.Errorf("can't setup HTTP client to %s: %w", rawUrl, err)
	}

	return &Client{
		baseUrl:    cleanedURL,
		httpClient: httpClient,
	}, nil
}

func runStatus(ctx context.Context) error {
	const url = "https://status.fly.io"
	var (
		cfg               = config.FromContext(ctx)
		getStatusEndpoint string
		getStatusKind     = flag.FirstArg(ctx)
	)

	switch getStatusKind {
	case "incidents":
		getStatusEndpoint = "api/v2/incidents/unresolved.json"
	case "maintenance":
		getStatusEndpoint = "api/v2/scheduled-maintenances/active.json"
	default:
		getStatusEndpoint = "api/v2/status.json"
	}

	if cfg.JSONOutput {
		httpClient, err := api.NewHTTPClient(logger.MaybeFromContext(ctx), http.DefaultTransport)
		if err != nil {
		    return err
		}
		res, err := httpClient.Get(url+getStatusEndpoint)
		if err != nil {
		    return err
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			err = api.ErrorFromResp(res)
			return fmt.Errorf("failed to retrieve status: %w", err)
		}

		var result = StatusPage{}
		if err = json.NewDecoder(res.Body).Decode(&result); err != nil {
			fmt.Println(err)
			return nil
		}
		out := iostreams.FromContext(ctx).Out
		return render.JSON(out, result)
	}

	w := iostreams.FromContext(ctx).ErrOut
	fmt.Fprintf(w, "opening %s ...\n", url)

	if err := open.Run(url); err != nil {
		return fmt.Errorf("failed opening %s: %w", url, err)
	}

	return nil
}
