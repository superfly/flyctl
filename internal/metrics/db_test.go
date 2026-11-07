package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
)

// captureStderr returns what fn writes to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	return string(out)
}

// Headless runs send metrics in-process, so anything SendMetrics prints lands
// in the user's output. Metrics are best-effort: a logged-out user or an
// unreachable collector must not produce warnings.
func TestSendMetricsIsSilentWithoutToken(t *testing.T) {
	ctx := config.NewContext(context.Background(), &config.Config{Tokens: new(tokens.Tokens)})

	var err error
	stderr := captureStderr(t, func() { err = SendMetrics(ctx, "[]") })

	if err != nil {
		t.Fatalf("SendMetrics() error = %v, want nil", err)
	}
	if stderr != "" {
		t.Fatalf("SendMetrics() wrote to stderr: %q", stderr)
	}
}

func TestSendMetricsIsSilentWhenSendFails(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer collector.Close()

	ctx := config.NewContext(context.Background(), &config.Config{
		Tokens:         new(tokens.Tokens),
		MetricsToken:   "metrics-token",
		MetricsBaseURL: collector.URL,
	})

	var err error
	stderr := captureStderr(t, func() { err = SendMetrics(ctx, "[]") })

	if err == nil {
		t.Fatal("SendMetrics() error = nil, want the failed send reported to the caller")
	}
	if stderr != "" {
		t.Fatalf("SendMetrics() wrote to stderr: %q", stderr)
	}
}
