package help

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/command"
)

func New(root *cobra.Command) *cobra.Command {
	cmd := command.New("help", "bad help", "", func(ctx context.Context) error {
		hf := root.HelpFunc()
		hf(root, nil)

		return nil
	})

	return cmd
}

func NewRootHelp() *cobra.Command {
	return command.New("help", "bad help", "", func(ctx context.Context) error {
		fmt.Println(docstrings.Get("flyctl").Long)
		return nil
	})
}
