package tigris

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func New() (cmd *cobra.Command) {
	const (
		short = "Provision and manage Tigris object storage buckets"
		long  = short + "\n"
	)

	cmd = command.New("storage", short, long, nil)
	cmd.Aliases = []string{"tigris"}
	cmd.AddCommand(create(), update(), list(), dashboard(), destroy(), status())

	return cmd
}

var SharedFlags = flag.Set{
	flag.String{
		Name:        "shadow-access-key",
		Description: "Shadow bucket access key",
	},
	flag.String{
		Name:        "shadow-secret-key",
		Description: "Shadow bucket secret key",
	},
	flag.String{
		Name:        "shadow-region",
		Description: "Shadow bucket region",
	},
	flag.String{
		Name:        "shadow-endpoint",
		Description: "Shadow bucket endpoint",
	},
	flag.String{
		Name:        "shadow-name",
		Description: "Shadow bucket name",
	},
	flag.Bool{
		Name:        "shadow-write-through",
		Description: "Write objects through to the shadow bucket",
	},
	flag.Bool{
		Name:        "accelerate",
		Hidden:      true,
		Description: "Cache objects on write in all regions",
	},
	flag.String{
		Name:        "website-domain-name",
		Description: "Domain name for website",
		Hidden:      true,
	},
	flag.Bool{
		Name:        "public",
		Shorthand:   "p",
		Description: "Objects in the bucket should be publicly accessible",
	},
}
