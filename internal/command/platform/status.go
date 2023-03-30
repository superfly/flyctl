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
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newStatus() (cmd *cobra.Command) {
	const (
		long = `Show current Fly platform status in a browser
`
		short = "Show current platform status"
	)

	cmd = command.New("status", short, long, runStatus)

	cmd.Args = cobra.NoArgs

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
	var cfg = config.FromContext(ctx)

	if cfg.JSONOutput {
		client, _ := createClient(ctx, url)
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", client.baseUrl, "api/v2/status.json"), nil)
		if err != nil {
			log.Fatal(err)
		}
		var res *http.Response

		if res, err = client.httpClient.Do(req); err != nil {
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
