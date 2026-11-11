package redis

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

// fakeGenqClient answers each GraphQL operation with canned JSON, keyed by
// operation name, and records the operations and variables it saw.
type fakeGenqClient struct {
	responses map[string]string
	errs      map[string]error
	opNames   []string
	vars      []map[string]any
}

func (c *fakeGenqClient) MakeRequest(_ context.Context, req *genq.Request, resp *genq.Response) error {
	c.opNames = append(c.opNames, req.OpName)

	var vars map[string]any
	encoded, err := json.Marshal(req.Variables)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(encoded, &vars); err != nil {
		return err
	}
	c.vars = append(c.vars, vars)

	if err := c.errs[req.OpName]; err != nil {
		return err
	}
	body, ok := c.responses[req.OpName]
	if !ok {
		return errors.New("unexpected operation " + req.OpName)
	}

	return json.Unmarshal([]byte(body), resp.Data)
}

const (
	getAddOnProdCache = `{"addOn":{"name":"prod-cache","privateIp":"fdaa::3","password":"pw","organization":{"slug":"prod"}}}`
	getAddOnStaging   = `{"addOn":{"name":"staging-cache","privateIp":"fdaa::4","password":"pw2","organization":{"slug":"staging"}}}`
	listAllAddOns     = `{"addOns":{"nodes":[
		{"name":"prod-cache","primaryRegion":"iad","organization":{"slug":"prod"}},
		{"name":"staging-cache","primaryRegion":"ord","organization":{"slug":"staging"}}]}}`
	listStagingAddOns = `{"organization":{"addOns":{"nodes":[
		{"name":"staging-cache","primaryRegion":"ord"}]}}}`
)

type pickerCalls struct {
	checkedOrg string
	options    []string
	prompted   bool
}

func (p *pickerCalls) checkOrg(err error) func(context.Context, string) error {
	return func(_ context.Context, slug string) error {
		p.checkedOrg = slug
		return err
	}
}

func (p *pickerCalls) choose(index int, err error) func(context.Context, []string) (int, error) {
	return func(_ context.Context, options []string) (int, error) {
		p.prompted = true
		p.options = options
		return index, err
	}
}

// orgJSON is a GetOrganization response. Personal orgs answer to the alias
// "personal" (slug) as well as their raw slug.
func orgJSON(slug, rawSlug string) string {
	return `{"organization":{"id":"o1","slug":"` + slug + `","rawSlug":"` + rawSlug + `"}}`
}

const getAddOnPersonal = `{"addOn":{"name":"my-cache","privateIp":"fdaa::5","password":"pw3","organization":{"slug":"personal"}}}`

func TestResolveRedisDatabaseByName(t *testing.T) {
	tests := []struct {
		name     string
		dbName   string
		org      string
		orgResp  string
		getAddOn string
		wantOps  []string
		wantErr  string
		wantSlug string
		wantIP   string
		wantPass string
	}{
		{
			name:     "name without --org: one lookup, no org check",
			dbName:   "prod-cache",
			getAddOn: getAddOnProdCache,
			wantOps:  []string{"GetAddOn"},
			wantSlug: "prod", wantIP: "fdaa::3", wantPass: "pw",
		},
		{
			name:     "name with matching --org",
			dbName:   "staging-cache",
			org:      "staging",
			orgResp:  orgJSON("staging", "staging"),
			getAddOn: getAddOnStaging,
			wantOps:  []string{"GetOrganization", "GetAddOn"},
			wantSlug: "staging", wantIP: "fdaa::4", wantPass: "pw2",
		},
		{
			name:     "--org given as raw personal slug matches the personal alias",
			dbName:   "my-cache",
			org:      "jane-doe",
			orgResp:  orgJSON("personal", "jane-doe"),
			getAddOn: getAddOnPersonal,
			wantOps:  []string{"GetOrganization", "GetAddOn"},
			wantSlug: "personal", wantIP: "fdaa::5", wantPass: "pw3",
		},
		{
			name:     "--org given as the personal alias",
			dbName:   "my-cache",
			org:      "personal",
			orgResp:  orgJSON("personal", "jane-doe"),
			getAddOn: getAddOnPersonal,
			wantOps:  []string{"GetOrganization", "GetAddOn"},
			wantSlug: "personal", wantIP: "fdaa::5", wantPass: "pw3",
		},
		{
			name:     "name in a different org than --org is rejected",
			dbName:   "prod-cache",
			org:      "staging",
			orgResp:  orgJSON("staging", "staging"),
			getAddOn: getAddOnProdCache,
			wantOps:  []string{"GetOrganization", "GetAddOn"},
			wantErr:  `Redis database "prod-cache" belongs to organization "prod", not "staging" (from --org)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeGenqClient{responses: map[string]string{"GetAddOn": tt.getAddOn, "GetOrganization": tt.orgResp}}
			calls := &pickerCalls{}

			db, err := resolveRedisDatabase(context.Background(), client, tt.dbName, tt.org,
				calls.checkOrg(nil), calls.choose(0, nil))

			// A named database never lists or prompts.
			assert.Equal(t, tt.wantOps, client.opNames)
			last := client.vars[len(client.vars)-1]
			assert.Equal(t, tt.dbName, last["name"])
			assert.Equal(t, "upstash_redis", last["provider"])
			assert.False(t, calls.prompted, "should not prompt when a name is given")

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, db)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSlug, db.Organization.Slug)
			assert.Equal(t, tt.wantIP, db.PrivateIp)
			assert.Equal(t, tt.wantPass, db.Password)
		})
	}
}

func TestResolveRedisDatabaseByNameUnknownOrg(t *testing.T) {
	client := &fakeGenqClient{errs: map[string]error{"GetOrganization": errors.New("Could not find Organization")}}
	calls := &pickerCalls{}

	_, err := resolveRedisDatabase(context.Background(), client, "prod-cache", "prdo", calls.checkOrg(nil), calls.choose(0, nil))

	// A typo in --org reads as "org not found", not as an ownership mismatch.
	require.EqualError(t, err, `failed to resolve organization "prdo": Could not find Organization`)
	assert.Equal(t, []string{"GetOrganization"}, client.opNames)
}

func TestResolveRedisDatabaseUnknownName(t *testing.T) {
	client := &fakeGenqClient{errs: map[string]error{"GetAddOn": errors.New("Could not find AddOn")}}
	calls := &pickerCalls{}

	_, err := resolveRedisDatabase(context.Background(), client, "nope", "", calls.checkOrg(nil), calls.choose(0, nil))

	require.EqualError(t, err, "Could not find AddOn")
	assert.False(t, calls.prompted)
}

func TestResolveRedisDatabasePicker(t *testing.T) {
	t.Run("no name, no --org: lists every org and connects to the choice", func(t *testing.T) {
		client := &fakeGenqClient{responses: map[string]string{
			"ListAddOns": listAllAddOns,
			"GetAddOn":   getAddOnStaging,
		}}
		calls := &pickerCalls{}

		db, err := resolveRedisDatabase(context.Background(), client, "", "", calls.checkOrg(nil), calls.choose(1, nil))

		require.NoError(t, err)
		assert.Equal(t, []string{"ListAddOns", "GetAddOn"}, client.opNames)
		assert.Equal(t, []string{"prod-cache (iad) prod", "staging-cache (ord) staging"}, calls.options)
		assert.Equal(t, "staging-cache", client.vars[1]["name"])
		assert.Empty(t, calls.checkedOrg)
		assert.Equal(t, "staging", db.Organization.Slug)
	})

	t.Run("no name, --org: checks the org and lists only its databases", func(t *testing.T) {
		client := &fakeGenqClient{responses: map[string]string{
			"ListOrganizationAddOns": listStagingAddOns,
			"GetAddOn":               getAddOnStaging,
		}}
		calls := &pickerCalls{}

		db, err := resolveRedisDatabase(context.Background(), client, "", "staging", calls.checkOrg(nil), calls.choose(0, nil))

		require.NoError(t, err)
		assert.Equal(t, "staging", calls.checkedOrg)
		assert.Equal(t, []string{"ListOrganizationAddOns", "GetAddOn"}, client.opNames)
		assert.Equal(t, "staging", client.vars[0]["orgSlug"])
		assert.Equal(t, []string{"staging-cache (ord) staging"}, calls.options)
		assert.Equal(t, "fdaa::4", db.PrivateIp)
	})

	t.Run("--org as raw personal slug: pick still connects (no post-pick ownership check)", func(t *testing.T) {
		// Works on master today: the list is scoped by the raw slug, but the
		// picked add-on reports its org as "personal".
		client := &fakeGenqClient{responses: map[string]string{
			"ListOrganizationAddOns": `{"organization":{"addOns":{"nodes":[{"name":"my-cache","primaryRegion":"iad"}]}}}`,
			"GetAddOn":               getAddOnPersonal,
		}}
		calls := &pickerCalls{}

		db, err := resolveRedisDatabase(context.Background(), client, "", "jane-doe", calls.checkOrg(nil), calls.choose(0, nil))

		require.NoError(t, err)
		assert.Equal(t, []string{"ListOrganizationAddOns", "GetAddOn"}, client.opNames)
		assert.Equal(t, "personal", db.Organization.Slug)
	})

	t.Run("unknown --org fails before listing", func(t *testing.T) {
		client := &fakeGenqClient{}
		calls := &pickerCalls{}

		_, err := resolveRedisDatabase(context.Background(), client, "", "nope", calls.checkOrg(errors.New("org not found")), calls.choose(0, nil))

		require.EqualError(t, err, "org not found")
		assert.Empty(t, client.opNames)
		assert.False(t, calls.prompted)
	})

	t.Run("no databases: clear error instead of an empty prompt", func(t *testing.T) {
		client := &fakeGenqClient{responses: map[string]string{"ListAddOns": `{"addOns":{"nodes":[]}}`}}
		calls := &pickerCalls{}

		_, err := resolveRedisDatabase(context.Background(), client, "", "", calls.checkOrg(nil), calls.choose(0, nil))

		require.ErrorContains(t, err, "no Redis databases found")
		assert.False(t, calls.prompted)
	})

	t.Run("non-interactive: error says to pass the name", func(t *testing.T) {
		client := &fakeGenqClient{responses: map[string]string{"ListAddOns": listAllAddOns}}
		calls := &pickerCalls{}

		_, err := resolveRedisDatabase(context.Background(), client, "", "", calls.checkOrg(nil),
			calls.choose(0, prompt.NonInteractiveError("prompt: non interactive")))

		require.ErrorContains(t, err, "pass the Redis database name as an argument")
		assert.True(t, prompt.IsNonInteractive(err))
		assert.Equal(t, []string{"ListAddOns"}, client.opNames)
	})

	t.Run("other prompt errors are returned as is", func(t *testing.T) {
		client := &fakeGenqClient{responses: map[string]string{"ListAddOns": listAllAddOns}}
		calls := &pickerCalls{}

		_, err := resolveRedisDatabase(context.Background(), client, "", "", calls.checkOrg(nil), calls.choose(0, errors.New("boom")))

		require.EqualError(t, err, "boom")
	})
}

func TestConnectAndProxyAcceptOptionalName(t *testing.T) {
	for name, cmd := range map[string]*cobra.Command{"connect": newConnect(), "proxy": newProxy()} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, name+" [name]", cmd.Use)
			assert.NoError(t, cmd.Args(cmd, nil), "no name must still work (interactive picker)")
			assert.NoError(t, cmd.Args(cmd, []string{"cache"}))
			assert.Error(t, cmd.Args(cmd, []string{"a", "b"}))
			// --org and --region stay registered so existing invocations keep parsing.
			assert.NotNil(t, cmd.Flags().Lookup("org"))
			assert.NotNil(t, cmd.Flags().Lookup("region"))
		})
	}
}

// parseRedisCmd parses args against the real command's flags and returns a
// context carrying them, the way command.New does before calling run.
func parseRedisCmd(t *testing.T, cmd *cobra.Command, args ...string) context.Context {
	t.Helper()
	require.NoError(t, cmd.ParseFlags(args))

	return flag.NewContext(context.Background(), cmd.Flags())
}

func TestRedisTarget(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flyOrg   string
		wantName string
		wantOrg  string
	}{
		{name: "no args", args: nil, wantName: "", wantOrg: ""},
		{name: "positional name reaches the resolver", args: []string{"cache"}, wantName: "cache"},
		{name: "--org with name", args: []string{"cache", "--org", "staging"}, wantName: "cache", wantOrg: "staging"},
		{name: "--org without name scopes the picker", args: []string{"-o", "staging"}, wantOrg: "staging"},
		{name: "FLY_ORG scopes the picker", flyOrg: "staging", wantOrg: "staging"},
		{name: "FLY_ORG does not reject a named database", args: []string{"cache"}, flyOrg: "staging", wantName: "cache"},
		{name: "--org beats FLY_ORG", args: []string{"cache", "--org", "prod"}, flyOrg: "staging", wantName: "cache", wantOrg: "prod"},
	}

	for _, tt := range tests {
		for cmdName, newCmd := range map[string]func() *cobra.Command{"connect": newConnect, "proxy": newProxy} {
			t.Run(cmdName+"/"+tt.name, func(t *testing.T) {
				t.Setenv("FLY_ORG", tt.flyOrg)
				ctx := parseRedisCmd(t, newCmd(), tt.args...)

				name, org := redisTarget(ctx)

				assert.Equal(t, tt.wantName, name)
				assert.Equal(t, tt.wantOrg, org)
			})
		}
	}
}

func TestConnectFailsFastWithoutRedisCli(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ios, _, _, _ := iostreams.Test()
	// No flags or API client in the context: if runConnect got past the
	// redis-cli check it would panic looking them up.
	ctx := iostreams.NewContext(context.Background(), ios)

	err := runConnect(ctx)

	require.ErrorContains(t, err, "could not find redis-cli")
}
