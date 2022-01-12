package volumes

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newCreate() *cobra.Command {
	const (
		long = `Create new volume for app. --region flag must be included to specify
region the volume exists in. --size flag is optional, defaults to 10,
sets the size as the number of gigabytes the volume will consume.`

		short = "Create new volume for app"
	)

	cmd := command.New("create", short, long, runCreate)

	return cmd
}

func runCreate(ctx context.Context) error {
	return nil
}
