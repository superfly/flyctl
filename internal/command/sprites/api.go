package sprites

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/iostreams"
)

func newAPI() *cobra.Command {
	const (
		short = "Make an authenticated Sprites API request with curl."

		long = `Call the Sprites API with curl, adding the API address and your token.

Paths starting with /v1/ are used as given. Other paths are resolved against
the target sprite when there is one, so /checkpoints becomes
/v1/sprites/<sprite>/checkpoints, and against /v1 otherwise.

Everything after the path is passed to curl.
`
	)

	cmd := newCommand("api [flags] <path> [curl options]", short, long, runAPI)
	cmd.Args = cobra.MinimumNArgs(1)
	cmd.Example = `  fly sprites api /v1/sprites
  fly sprites api -s my-sprite /checkpoints
  fly sprites api /v1/sprites -X POST -d '{"name":"my-sprite"}'`

	// Stop parsing flags at the path so curl's options reach curl.
	cmd.Flags().SetInterspersed(false)

	return cmd
}

func runAPI(ctx context.Context, s *session) error {
	ios := iostreams.FromContext(ctx)

	args := flag.Args(ctx)
	path, curlArgs := args[0], args[1:]

	// The standalone CLI separated curl's options with "--".
	if len(curlArgs) > 0 && curlArgs[0] == "--" {
		curlArgs = curlArgs[1:]
	}

	// The sprite is optional here: organization-level paths have none.
	sprite, err := s.namedSprite(ctx, "")
	if err != nil && !errors.Is(err, errNoSprite) {
		return err
	}

	url := s.APIURL + apiPath(path, sprite)

	curl, err := exec.LookPath("curl")
	if err != nil {
		return flyerr.WithSuggestion(errors.New("curl is not installed"), "Install curl to use this command.")
	}

	// Hand curl the token in a file rather than on its command line, where
	// other users on the machine could read it from the process list.
	header, err := os.CreateTemp("", "fly-sprites-api-*")
	if err != nil {
		return fmt.Errorf("failed to create a temporary file: %w", err)
	}
	defer os.Remove(header.Name()) //nolint:errcheck

	_, werr := header.WriteString("Authorization: Bearer " + s.Token + "\n")
	if cerr := header.Close(); werr == nil {
		werr = cerr
	}

	if werr != nil {
		return fmt.Errorf("failed to write a temporary file: %w", werr)
	}

	fmt.Fprintf(ios.ErrOut, "URL: %s\n\n", url)

	cmd := exec.CommandContext(ctx, curl, append([]string{"-H", "@" + header.Name(), url}, curlArgs...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = ios.Out
	cmd.Stderr = ios.ErrOut

	var exitErr *exec.ExitError

	switch err := cmd.Run(); {
	case err == nil:
		return nil
	case errors.As(err, &exitErr):
		// curl has already reported what went wrong.
		return &flyerr.ExitCodeError{Code: exitErr.ExitCode()}
	default:
		return err
	}
}

// apiPath maps the path given to `api` onto an API path.
func apiPath(path, sprite string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	switch {
	case path == "/v1" || strings.HasPrefix(path, "/v1/"):
		return path
	case sprite != "":
		return "/v1/sprites/" + sprite + path
	default:
		return "/v1" + path
	}
}
