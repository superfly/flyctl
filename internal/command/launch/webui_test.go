package launch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/logger"
)

// cliSessionServer answers session polls with status, the way the API answers
// 404 until someone confirms the launch page.
func cliSessionServer(t *testing.T, status int, body string) {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(ts.Close)

	fly.SetBaseURL(ts.URL)
	t.Cleanup(func() { fly.SetBaseURL("") })
}

func TestWaitForCLISessionReportsWhyTheWaitEnded(t *testing.T) {
	cliSessionServer(t, http.StatusNotFound, "")
	log := logger.New(io.Discard, logger.Error, false)

	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := waitForCLISession(ctx, log, io.Discard, "sess1")

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected the deadline, got %v", err)
		}
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)

		_, err := waitForCLISession(ctx, log, io.Discard, "sess1")

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected the cancellation, got %v", err)
		}
	})

	t.Run("already ended", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := waitForCLISession(ctx, log, io.Discard, "sess1")

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected the cancellation, got %v", err)
		}
	})
}

func TestWaitForCLISessionReturnsConfirmedSession(t *testing.T) {
	cliSessionServer(t, http.StatusOK, `{"id":"sess1","metadata":{"name":"my-app"}}`)

	var out bytes.Buffer
	session, err := waitForCLISession(context.Background(), logger.New(io.Discard, logger.Error, false), &out, "sess1")

	if err != nil {
		t.Fatal(err)
	}
	if session.ID != "sess1" || session.Metadata["name"] != "my-app" {
		t.Fatalf("unexpected session: %+v", session)
	}
}
