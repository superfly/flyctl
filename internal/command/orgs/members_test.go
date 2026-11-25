package orgs

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

var errOrgLookup = errors.New("org lookup failed")

// A failure before the membership changes must not look like success.
func TestInviteAndRemoveReturnErrors(t *testing.T) {
	t.Setenv("FLY_ORG", "")

	for _, sub := range []struct {
		name string
		cmd  *cobra.Command
		run  func(context.Context) error
	}{
		{"invite", newInvite(), runInvite},
		{"remove", newRemove(), runRemove},
	} {
		for _, tc := range []struct {
			name  string
			args  []string
			check func(*testing.T, error)
		}{
			{"unknown org", []string{"typo", "bob@example.com"}, func(t *testing.T, err error) { require.ErrorIs(t, err, errOrgLookup) }},
			{"no org", nil, func(t *testing.T, err error) { require.True(t, prompt.IsNonInteractive(err), "got %v", err) }},
			{"no email", []string{"acme"}, func(t *testing.T, err error) { require.True(t, prompt.IsNonInteractive(err), "got %v", err) }},
		} {
			t.Run(sub.name+" "+tc.name, func(t *testing.T) {
				io, _, _, _ := iostreams.Test()
				ctx := iostreams.NewContext(t.Context(), io)
				ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
					GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
						if slug != "acme" {
							return nil, errOrgLookup
						}

						return &uiex.Organization{Slug: slug}, nil
					},
				})
				ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
					GetDetailedOrganizationBySlugFunc: func(context.Context, string) (*fly.OrganizationDetails, error) {
						return &fly.OrganizationDetails{}, nil
					},
				})

				require.NoError(t, sub.cmd.Flags().Parse(tc.args))
				tc.check(t, sub.run(flag.NewContext(ctx, sub.cmd.Flags())))
			})
		}
	}
}

func TestRemoveReturnsOrganizationDetailsError(t *testing.T) {
	t.Setenv("FLY_ORG", "")

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(t.Context(), io)
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
			return &uiex.Organization{Slug: slug}, nil
		},
	})
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		GetDetailedOrganizationBySlugFunc: func(context.Context, string) (*fly.OrganizationDetails, error) {
			return nil, errOrgLookup
		},
	})

	cmd := newRemove()
	require.NoError(t, cmd.Flags().Parse([]string{"acme", "bob@example.com"}))
	require.ErrorIs(t, runRemove(flag.NewContext(ctx, cmd.Flags())), errOrgLookup)
}
