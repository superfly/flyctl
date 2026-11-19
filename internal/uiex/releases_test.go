package uiex

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flyerr"
)

// Overdue invoices are an account state the API explains to the user, so the
// refusal is marked expected and kept out of Sentry. Other refusals are not.
func TestCreateReleaseMarksOverdueInvoicesExpected(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		expected bool
	}{
		{"overdue invoices", http.StatusForbidden, "Your account has overdue invoices. Please update your payment information: https://fly.io/dashboard/example/billing", true},
		{"another 403", http.StatusForbidden, "Not authorized to deploy this app.", false},
		{"overdue invoices text in a 500", http.StatusInternalServerError, "error while checking for overdue invoices", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := client.CreateRelease(ctx, CreateReleaseRequest{})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.body)
			require.Equal(t, tc.expected, flyerr.IsExpected(err))
		})
	}
}
