//go:build !windows

package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/AlecAivazis/survey/v2/terminal"
	genq "github.com/Khan/genqlient/graphql"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

const (
	keyEnter = "\r"
	keyDown  = "\x1b[B"
	keyCtrlC = "\x03"
)

// keyInput returns one keystroke per Read. Survey takes an escape that ends a
// read for the Esc key, so an arrow key's bytes must arrive together, which
// mock.PromptIO's one-byte reads don't allow.
type keyInput struct{ keys []string }

func (k *keyInput) Read(p []byte) (int, error) {
	if len(k.keys) == 0 {
		return 0, io.EOF
	}

	n := copy(p, k.keys[0])
	k.keys = k.keys[1:]

	return n, nil
}

func (*keyInput) Fd() uintptr  { return ^uintptr(0) }
func (*keyInput) Close() error { return nil }

// pick returns the keystrokes that choose the option at index in a list.
func pick(index int) []string {
	var keys []string
	for range index {
		keys = append(keys, keyDown)
	}

	return append(keys, keyEnter)
}

// typed returns the keystrokes that answer a text prompt. Survey first asks
// the terminal for its size and cursor position.
func typed(answer string) []string {
	return []string{"\x1b[24;80R", "\x1b[1;1R", answer}
}

type createInput struct {
	Type    string         `json:"type"`
	PlanID  string         `json:"planId"`
	AppID   string         `json:"appId"`
	Options map[string]any `json:"options"`
}

// runCreateWithKeys runs the command for an app, answering its prompts with
// keys. It returns the names of the GraphQL operations sent, the create
// requests, the prompts' output and the command's error.
func runCreateWithKeys(t *testing.T, keys []string) (ops []string, created []createInput, output string, err error) {
	t.Helper()

	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			OperationName string `json:"operationName"`
			Variables     struct {
				Input createInput `json:"input"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		mu.Lock()
		defer mu.Unlock()
		ops = append(ops, request.OperationName)

		w.Header().Set("Content-Type", "application/json")
		switch request.OperationName {
		case "GetAddOnProvider":
			fmt.Fprint(w, `{"data":{"addOnProvider":{"id":"provider","name":"upstash_vector","displayName":"Upstash Vector","resourceName":"index"}}}`)
		case "AgreedToProviderTos":
			fmt.Fprint(w, `{"data":{"viewer":{"__typename":"User","agreedToProviderTos":true}}}`)
		case "GetAppWithAddons":
			fmt.Fprint(w, `{"data":{"app":{"id":"app-id","name":"my-app","organization":{"id":"org"},"addOns":{"nodes":[]}}}}`)
		case "CreateExtension":
			created = append(created, request.Variables.Input)
			fmt.Fprint(w, `{"data":{"createAddOn":{"addOn":{"id":"index","name":"my-index","environment":null}}}}`)
		default:
			t.Errorf("unexpected GraphQL operation %q", request.OperationName)
			http.Error(w, "unexpected operation", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("yes", false, "")
	ctx := flag.NewContext(context.Background(), flags)
	ctx = appconfig.WithName(ctx, "my-app")

	streams, stdout, stderr := mock.PromptIO()
	input := &keyInput{keys: keys}
	streams.In = input
	ctx = iostreams.NewContext(ctx, streams)
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{GenqClientFunc: func() genq.Client {
		return genq.NewClient(server.URL, server.Client())
	}})

	err = runCreate(ctx)
	require.Empty(t, input.keys, "keystrokes left unread: a prompt was skipped")

	return ops, created, stdout.String() + stderr.String(), err
}

func join(keys ...[]string) []string {
	var all []string
	for _, k := range keys {
		all = append(all, k...)
	}

	return all
}

func TestRunCreateProvisionsPickedModel(t *testing.T) {
	for i, model := range embeddingModels {
		t.Run(model.Name, func(t *testing.T) {
			_, created, _, err := runCreateWithKeys(t, join(pick(0), pick(i+1)))
			require.NoError(t, err)
			require.Len(t, created, 1)
			require.Equal(t, "upstash_vector", created[0].Type)
			require.Equal(t, "app-id", created[0].AppID)
			require.NotEmpty(t, created[0].PlanID)
			require.Equal(t, map[string]any{
				"similarity_function": similarityFunctions[0].Identifier,
				"embedding_model":     model.Identifier,
				"dimension_count":     float64(model.Dimensions),
			}, created[0].Options)
		})
	}
}

func TestRunCreateProvisionsPickedSimilarityFunction(t *testing.T) {
	for i, function := range similarityFunctions {
		t.Run(function.Name, func(t *testing.T) {
			_, created, _, err := runCreateWithKeys(t, join(pick(i), pick(1)))
			require.NoError(t, err)
			require.Len(t, created, 1)
			require.Equal(t, function.Identifier, created[0].Options["similarity_function"])
		})
	}
}

func TestRunCreateWithOwnEmbeddings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keys  []string
		wants int
	}{
		{"default dimensions", typed(keyEnter), 128},
		{"typed dimensions", typed("768" + keyEnter), 768},
		{"asks again after a non-number", join(typed("many"+keyEnter), typed("256"+keyEnter)), 256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, created, output, err := runCreateWithKeys(t, join(pick(0), pick(0), tc.keys))
			require.NoError(t, err)
			require.Len(t, created, 1)
			require.Equal(t, map[string]any{
				"similarity_function": similarityFunctions[0].Identifier,
				"dimension_count":     float64(tc.wants),
			}, created[0].Options)
			require.Equal(t, strings.Contains(tc.name, "non-number"), strings.Contains(output, "must be an integer"))
		})
	}
}

// Interrupting any prompt must stop the command before it sends a request.
func TestRunCreateInterruptedPromptSendsNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys []string
	}{
		{"similarity function", []string{keyCtrlC}},
		{"embedding model", join(pick(0), []string{keyCtrlC})},
		{"dimensions", join(pick(0), pick(0), typed(keyCtrlC))},
		{"dimensions, partly typed", join(pick(0), pick(0), typed("76"+keyCtrlC))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops, _, _, err := runCreateWithKeys(t, tc.keys)
			require.ErrorIs(t, err, terminal.InterruptErr)
			require.Empty(t, ops, "a request was sent after the prompt was interrupted")
		})
	}
}
