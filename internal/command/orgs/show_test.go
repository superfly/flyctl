package orgs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// runShowWith runs `fly orgs show personal` against a GraphQL client that
// answers the credit balance query with respond.
func runShowWith(t *testing.T, jsonOutput bool, respond mock.GraphQLClient) (stdout, stderr string, err error) {
	t.Helper()
	t.Setenv("FLY_ORG", "")

	io, _, out, errOut := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
			return &uiex.Organization{Slug: slug}, nil
		},
	})
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		GetDetailedOrganizationBySlugFunc: func(_ context.Context, slug string) (*fly.OrganizationDetails, error) {
			org := &fly.OrganizationDetails{Name: "Example Org", Slug: slug, Type: "PERSONAL", ViewerRole: "admin"}
			org.Members.Edges = []fly.OrganizationMembershipEdge{{Node: fly.User{Name: "Ada", Email: "ada@example.com"}, Role: "admin"}}
			return org, nil
		},
		GenqClientFunc: func() genq.Client { return respond },
	})
	flags := pflag.NewFlagSet("show", pflag.ContinueOnError)
	require.NoError(t, flags.Parse([]string{"personal"}))
	ctx = flagctx.NewContext(ctx, flags)

	err = runShow(ctx)

	return out.String(), errOut.String(), err
}

func creditBalanceAnswer(t *testing.T, data string) mock.GraphQLClient {
	return func(_ context.Context, req *genq.Request, resp *genq.Response) error {
		assert.Equal(t, "GetOrganizationCreditBalance", req.OpName)
		vars, err := json.Marshal(req.Variables)
		require.NoError(t, err)
		assert.JSONEq(t, `{"slug":"personal"}`, string(vars))

		return json.Unmarshal([]byte(data), resp.Data)
	}
}

func decodeJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))

	return got
}

func TestShowPrintsPurchasedCreditBalance(t *testing.T) {
	stdout, stderr, err := runShowWith(t, false, creditBalanceAnswer(t, `{"organization":{"creditBalance":123456}}`))
	require.NoError(t, err)

	assert.Contains(t, stdout, "Type            : PERSONAL")
	assert.Contains(t, stdout, "Purchased credit: $1,234.56")
	assert.Empty(t, stderr)
}

func TestShowJSONIncludesPurchasedCreditBalance(t *testing.T) {
	stdout, _, err := runShowWith(t, true, creditBalanceAnswer(t, `{"organization":{"creditBalance":1000}}`))
	require.NoError(t, err)

	got := decodeJSON(t, stdout)
	assert.Equal(t, float64(1000), got["PurchasedCreditBalanceCents"])
	assert.Equal(t, "personal", got["Slug"])
	assert.Equal(t, "Example Org", got["Name"])
	assert.Contains(t, got, "Members")
}

// Money owed is a negative balance, which the billing page shows as $0.
func TestShowMoneyOwedReadsAsZero(t *testing.T) {
	answer := `{"organization":{"creditBalance":-700}}`

	stdout, _, err := runShowWith(t, false, creditBalanceAnswer(t, answer))
	require.NoError(t, err)
	assert.Contains(t, stdout, "Purchased credit: $0.00")

	stdout, _, err = runShowWith(t, true, creditBalanceAnswer(t, answer))
	require.NoError(t, err)
	assert.Equal(t, float64(0), decodeJSON(t, stdout)["PurchasedCreditBalanceCents"])
}

// The rest of the org still shows when the balance can't be read, such as
// against an API that doesn't have the field yet.
func TestShowWarnsWhenCreditBalanceFails(t *testing.T) {
	fail := func(context.Context, *genq.Request, *genq.Response) error {
		return errors.New("Internal server error")
	}

	stdout, stderr, err := runShowWith(t, false, fail)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Slug            : personal")
	assert.Contains(t, stdout, "ada@example.com")
	assert.NotContains(t, stdout, "Purchased credit")
	assert.Contains(t, stderr, "Couldn't get the purchased credit balance for organization personal")
	assert.Contains(t, stderr, "Internal server error")

	stdout, stderr, err = runShowWith(t, true, fail)
	require.NoError(t, err)
	got := decodeJSON(t, stdout)
	assert.Equal(t, "personal", got["Slug"])
	assert.NotContains(t, got, "PurchasedCreditBalanceCents")
	assert.Contains(t, stderr, "Couldn't get the purchased credit balance")
}

func TestFormatCredit(t *testing.T) {
	for cents, want := range map[int]string{
		0:         "$0.00",
		5:         "$0.05",
		1000:      "$10.00",
		123456:    "$1,234.56",
		100000000: "$1,000,000.00",
	} {
		assert.Equal(t, want, formatCredit(cents), cents)
	}
}
