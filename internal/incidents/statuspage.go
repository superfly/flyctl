package incidents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/iostreams"
)

type StatusPageApiResponse struct {
	Incidents []Incident `json:"incidents"`
}

type Incident struct {
	Components []Component `json:"components"`
	CreatedAt  string      `json:"created_at"`
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	ResolvedAt string      `json:"resolved_at"`
	StartedAt  string      `json:"started_at"`
	Status     string      `json:"status"`
	UpdatedAt  string      `json:"updated_at"`
}

type Component struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func getStatuspageUnresolvedIncidentsUrl() string {
	url := os.Getenv("FLY_STATUSPAGE_UNRESOLVED_INCIDENTS_URL")
	if url != "" {
		return url
	}

	return "https://incidents.flyio.net/v1/incidents"
}

func QueryStatuspageIncidents(ctx context.Context) {

	logger := logger.FromContext(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	logger.Debug("started querying for statuspage incidents")

	statusCh := make(chan *StatusPageApiResponse, 1)
	statusCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	go func() {
		defer cancel()
		defer close(statusCh)
		response, err := StatuspageIncidentsRequest(statusCtx)
		if err != nil {
			logger.Debugf("failed querying for Statuspage incidents: %v", err)
		}
		statusCh <- response
	}()

	task.FromContext(ctx).RunFinalizer(func(parent context.Context) {
		cancel()
		select {
		case incidents := <-statusCh:
			if incidents == nil {
				break
			}

			logger.Debugf("querying for statuspage incidents resulted to %v", incidents)
			incidentCount := len(incidents.Incidents)
			if incidentCount > 0 {
				fmt.Fprintln(io.ErrOut, colorize.WarningIcon(),
					colorize.Yellow("WARNING: There are active incidents. Please check `fly incidents list` or visit https://status.flyio.net\n"),
				)
				break
			}
		}
	})
}

func StatuspageIncidentsRequest(ctx context.Context) (*StatusPageApiResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", getStatuspageUnresolvedIncidentsUrl(), http.NoBody)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close() // skipcq: GO-S2307

	if response.StatusCode != http.StatusOK {
		return nil, nil
	}

	var apiResponse StatusPageApiResponse
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("error: %s", err)
	}

	return &apiResponse, nil
}
