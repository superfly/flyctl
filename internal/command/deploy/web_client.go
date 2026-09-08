package deploy

import (
	"github.com/superfly/flyctl/logs"
)

// webClient is a subset of web API that is needed for the deploy package.
type webClient interface {
	logs.WebClient
	blueGreenWebClient
}
