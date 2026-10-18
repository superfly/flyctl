package extensions_core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/iostreams"
)

func TestProvisionExtensionBucketName(t *testing.T) {
	for _, tc := range []struct {
		name         string
		flagName     string
		overrideName *string
		appName      string
		nameSuffix   string
		provider     string
		wantName     string
		wantError    string
	}{
		{name: "reported long name", flagName: "afdshjfkjdhakjfhdsjkfhkjdfhaskjhfjdksafhjkahfkjdhsafkjhdakjfhskjfhakjfdhkjdsahkj", wantError: "between 3 and 63"},
		{name: "one character", flagName: "a", wantError: "between 3 and 63"},
		{name: "two characters", flagName: "ab", wantError: "between 3 and 63"},
		{name: "64 characters", flagName: strings.Repeat("a", 64), wantError: "between 3 and 63"},
		{name: "uppercase", flagName: "My-bucket", wantError: "lowercase"},
		{name: "underscore", flagName: "my_bucket", wantError: "lowercase"},
		{name: "space", flagName: "my bucket", wantError: "lowercase"},
		{name: "unicode", flagName: "bücket", wantError: "lowercase"},
		{name: "leading hyphen", flagName: "-bucket", wantError: "begin and end"},
		{name: "trailing hyphen", flagName: "bucket-", wantError: "begin and end"},
		{name: "leading period", flagName: ".bucket", wantError: "begin and end"},
		{name: "trailing period", flagName: "bucket.", wantError: "begin and end"},
		{name: "adjacent periods", flagName: "my..bucket", wantError: "adjacent periods"},
		{name: "reserved prefix", flagName: "xn--bucket", wantError: "xn--"},
		{name: "reserved suffix", flagName: "bucket-s3alias", wantError: "-s3alias"},
		{name: "IP address", flagName: "192.168.1.1", wantError: "IP address"},
		{name: "minimum length", flagName: "a-1", wantName: "a-1"},
		{name: "maximum length", flagName: strings.Repeat("a", 63), wantName: strings.Repeat("a", 63)},
		{name: "periods", flagName: "my.bucket", wantName: "my.bucket"},
		{name: "period next to hyphen", flagName: "my.-bucket", wantName: "my.-bucket"},
		{name: "numeric", flagName: "123", wantName: "123"},
		{name: "generated name"},
		{name: "invalid launch override", overrideName: new("ab"), wantError: "between 3 and 63"},
		{name: "override replaces invalid flag", flagName: "ab", overrideName: new("my-bucket"), wantName: "my-bucket"},
		{name: "empty launch override generates name", flagName: "ab", appName: "my-app", nameSuffix: "storage", overrideName: new("")},
		{name: "invalid app derived name", appName: strings.Repeat("a", 60), nameSuffix: "storage", wantError: "between 3 and 63"},
		{name: "valid app derived name", appName: "my-app", nameSuffix: "storage", wantName: "my-app-storage"},
		{name: "other providers keep their naming rules", provider: "upstash_vector", flagName: "My_Index", wantName: "My_Index"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := tc.provider
			if provider == "" {
				provider = "tigris"
			}
			var createCalls int
			var createdInput gql.CreateAddOnInput
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					OperationName string `json:"operationName"`
					Variables     struct {
						Input gql.CreateAddOnInput `json:"input"`
					} `json:"variables"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					http.Error(w, err.Error(), http.StatusBadRequest)

					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch request.OperationName {
				case "GetAddOnProvider":
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"addOnProvider": gql.ExtensionProviderData{
						Id: "provider", Name: provider, DisplayName: provider, ResourceName: "bucket",
						SelectName: true, NameSuffix: tc.nameSuffix, ProvisioningInstructions: "fly storage list",
					}}})
				case "AgreedToProviderTos":
					fmt.Fprint(w, `{"data":{"viewer":{"__typename":"User","agreedToProviderTos":true}}}`)
				case "GetOrganization":
					fmt.Fprint(w, `{"data":{"organization":{"id":"org","name":"test-org","slug":"test-org"}}}`)
				case "GetAppWithAddons":
					fmt.Fprintf(w, `{"data":{"app":{"id":"app","name":%q,"organization":{"id":"org"},"addOns":{"nodes":[]}}}}`, tc.appName)
				case "CreateExtension":
					createCalls++
					createdInput = request.Variables.Input
					if tc.wantError != "" {
						fmt.Fprint(w, `{"errors":[{"message":"The specified bucket is not valid."}]}`)

						return
					}
					fmt.Fprint(w, `{"data":{"createAddOn":{"addOn":{"id":"bucket","name":"my-bucket","environment":null}}}}`)
				default:
					t.Errorf("unexpected GraphQL operation %q", request.OperationName)
					http.Error(w, "unexpected operation", http.StatusBadRequest)
				}
			}))
			defer server.Close()

			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.String("name", tc.flagName, "")
			flags.Bool("yes", true, "")
			ctx := flag.NewContext(context.Background(), flags)
			streams, _, _, _ := iostreams.Test()
			ctx = iostreams.NewContext(ctx, streams)
			ctx = flyutil.NewContextWithClient(ctx, &mock.Client{GenqClientFunc: func() genq.Client {
				return genq.NewClient(server.URL, server.Client())
			}})

			_, err := ProvisionExtension(ctx, ExtensionParams{
				Provider: provider, Organization: &uiex.Organization{Slug: "test-org"},
				OverrideName: tc.overrideName, AppName: tc.appName,
			})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.ErrorContains(t, err, "https://www.tigrisdata.com/docs/buckets/bucket-rules/")
				require.Zero(t, createCalls, "invalid names must not reach the creation API")

				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, createCalls)
			require.Equal(t, tc.wantName, createdInput.Name)
			require.Equal(t, gql.AddOnType(provider), createdInput.Type)
		})
	}
}
