package cmd

// import (
// 	"context"

// 	"github.com/spf13/cobra"
// 	"github.com/superfly/flyctl/internal/build/imgsrc"
// 	"github.com/superfly/flyctl/internal/client"
// )

// type BuildOptions struct {
// 	Client     *client.Client
// 	LocalOnly  bool
// 	RemoteOnly bool
// }

// func newBuildCommand(client *client.Client) *Command {
// 	cmd := &cobra.Command{
// 		Use:   "build",
// 		Short: "Build source code into an OCI image",
// 		Long:  "Build source code into an OCI image",
// 	}

// 	opts := &BuildOptions{}
// 	cmd.Flags().BoolVar(&opts.LocalOnly, "local-only", false, "Only perform builds locally using the local docker daemon")
// 	cmd.Flags().BoolVar(&opts.RemoteOnly, "remote-only", false, "Perform builds remotely without using the local docker daemon")

// 	// cmd.RunE = func(cmd *cobra.Command, args []string) error {

// 	// }

// 	// cmd.RunE =

// 	// deployStrings := docstrings.Get("deploy")
// 	// cmd := BuildCommandKS(nil, runDeploy, deployStrings, client, workingDirectoryFromArg(0), requireSession, requireAppName)
// 	// cmd.AddStringFlag(StringFlagOpts{
// 	// 	Name:        "image",
// 	// 	Shorthand:   "i",
// 	// 	Description: "Image tag or id to deploy",
// 	// })
// 	// cmd.AddBoolFlag(BoolFlagOpts{
// 	// 	Name:        "detach",
// 	// 	Description: "Return immediately instead of monitoring deployment progress",
// 	// })
// 	// cmd.AddBoolFlag(BoolFlagOpts{
// 	// 	Name:   "build-only",
// 	// 	Hidden: true,
// 	// })
// 	// cmd.AddBoolFlag(BoolFlagOpts{
// 	// 	Name:        "remote-only",
// 	// 	Description: "Perform builds remotely without using the local docker daemon",
// 	// })
// 	// cmd.AddBoolFlag(BoolFlagOpts{
// 	// 	Name:        "local-only",
// 	// 	Description: "Only perform builds locally using the local docker daemon",
// 	// })
// 	// cmd.AddStringFlag(StringFlagOpts{
// 	// 	Name:        "strategy",
// 	// 	Description: "The strategy for replacing running instances. Options are canary, rolling, bluegreen, or immediate. Default is canary",
// 	// })
// 	// cmd.AddStringFlag(StringFlagOpts{
// 	// 	Name:        "dockerfile",
// 	// 	Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
// 	// })
// 	// cmd.AddStringSliceFlag(StringSliceFlagOpts{
// 	// 	Name:        "build-arg",
// 	// 	Description: "Set of build time variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
// 	// })
// 	// cmd.AddStringSliceFlag(StringSliceFlagOpts{
// 	// 	Name:        "env",
// 	// 	Shorthand:   "e",
// 	// 	Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
// 	// })
// 	// cmd.AddStringFlag(StringFlagOpts{
// 	// 	Name:        "image-label",
// 	// 	Description: "Image label to use when tagging and pushing to the fly registry. Defaults to \"deployment-{timestamp}\".",
// 	// })

// 	// cmd.Command.Args = cobra.MaximumNArgs(1)

// 	// return cmd
// 	return &Command{cmd}
// }

// func runBuild(ctx context.Context, opts BuildOptions) error {

// 	imgsrc.NewResolver(ctx)
// }
