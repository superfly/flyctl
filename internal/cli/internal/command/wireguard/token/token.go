// Package token implements the wireguard token command chain.
package token

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
)

func New() (cmd *cobra.Command) {
	const (
		short = "Commands that manage WireGuard delegated access tokens"
		long  = short + "n"
		usage = "token <command>"
	)

	// TODO: list should also accept the --org param

	cmd = command.New(usage, short, long, nil)

	cmd.AddCommand(
		newList(),
		newCreate(),
		newDelete(),
	)

	return
}

func nameFromFirstArgOrPrompt(ctx context.Context) (name string, err error) {
	if name = flag.FirstArg(ctx); name == "" {
		err = prompt.String(ctx, &name, "Enter WireGuard token name:", "", true)
	}

	return
}

func request(ctx context.Context, method, path, token string, data interface{}) (*http.Response, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(data); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://fly.io/api/v3/wire_guard_peers/%s", path)

	req, err := http.NewRequestWithContext(ctx, method, url, &buf)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")

	return http.DefaultClient.Do(req)
}
