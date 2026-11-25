package incidents

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatuspageIncidentsRequest(t *testing.T) {
	status := http.StatusServiceUnavailable
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"incidents":[{"id":"incident"}]}`))
	}))
	defer srv.Close()
	t.Setenv("FLY_STATUSPAGE_UNRESOLVED_INCIDENTS_URL", srv.URL)

	_, err := StatuspageIncidentsRequest(t.Context())
	require.ErrorContains(t, err, "503")

	status = http.StatusOK
	resp, err := StatuspageIncidentsRequest(t.Context())
	require.NoError(t, err)
	require.Equal(t, "incident", resp.Incidents[0].ID)
}
