package launch

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/iostreams"
)

func TestHeadlessLaunchBillingErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  uiex.BillingStatus
		message string
	}{
		{"trial ended", uiex.BillingStatusTrialEnded, "the trial for organization example-org has ended; a payment method is required to launch apps"},
		{"payment method missing", uiex.BillingStatusSourceRequired, "organization example-org needs a payment method to launch apps"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), ios)
			err := checkBillingStatus(ctx, &uiex.Organization{Slug: "example-org", BillingStatus: tc.status}, false)
			require.EqualError(t, err, tc.message)
			require.Equal(t, "Add one at https://fly.io/dashboard/example-org/billing, then run 'fly launch' again.", flyerr.GetErrorSuggestion(err))
			require.Empty(t, stdout.String(), "headless billing checks must not prompt or open a browser")
			require.Empty(t, stderr.String(), "the caller renders the returned error")
		})
	}
}

func TestOverdueLaunchBillingRemainsAdvisory(t *testing.T) {
	for _, status := range []uiex.BillingStatus{uiex.BillingStatusPastDue, uiex.BillingStatusDelinquent} {
		for _, canPrompt := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/prompt=%t", status, canPrompt), func(t *testing.T) {
				ios, _, stdout, stderr := iostreams.Test()
				ctx := iostreams.NewContext(context.Background(), ios)
				err := checkBillingStatus(ctx, &uiex.Organization{Slug: "example-org", BillingStatus: status}, canPrompt)
				require.NoError(t, err, "billing status alone must not block API-permitted operations")
				require.Empty(t, stdout.String())
				require.Equal(t, "Warning: organization example-org has overdue invoices. Review billing and pay any outstanding invoices at https://fly.io/dashboard/example-org/billing/invoices. Some billable operations may be blocked until billing is resolved.\n", stderr.String())
			})
		}
	}
}

func TestHeadlessLaunchOtherBillingStatusesUnchanged(t *testing.T) {
	for _, status := range []uiex.BillingStatus{uiex.BillingStatusCurrent, uiex.BillingStatusTrialActive, uiex.BillingStatusSuspended, "FUTURE_STATUS"} {
		t.Run(string(status), func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), ios)
			require.NoError(t, checkBillingStatus(ctx, &uiex.Organization{Slug: "example-org", BillingStatus: status}, false))
			require.Empty(t, stdout.String())
			require.Empty(t, stderr.String())
		})
	}
}
