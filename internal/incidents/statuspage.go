package incidents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
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

	return "https://status.flyio.net/api/v2/incidents/unresolved.json"
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
