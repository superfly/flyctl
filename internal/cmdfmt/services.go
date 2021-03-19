package cmdfmt

import (
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func PrintServicesList(s *iostreams.IOStreams, services []api.Service) {
	fmt.Fprintln(s.Out, aurora.Bold("Services"))
	for _, svc := range services {
		fmt.Fprintln(s.Out, svc.Description)
	}
}
