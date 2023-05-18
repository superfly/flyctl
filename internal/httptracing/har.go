package httptracing

import (
	"encoding/json"
	"github.com/haileys/go-harlog"
	"github.com/superfly/flyctl/terminal"
	"net/http"
	"os"
)

type harOpt struct {
	Path      string
	Container *harlog.HARContainer
}

var har *harOpt

func Init() {
	if path := os.Getenv("FLYCTL_OUTPUT_HAR"); path != "" {
		har = &harOpt{
			Path:      path,
			Container: harlog.NewHARContainer(),
		}
	}
}

func Finish() {
	if har == nil {
		return
	}

	harJson, err := json.MarshalIndent(har.Container, "", "    ")
	if err != nil {
		terminal.Warnf("error serializing HAR: %v\n", err)
		return
	}

	err = os.WriteFile(har.Path, harJson, 0644)
	if err != nil {
		terminal.Warnf("error writing HAR: %v\n", err)
	}
}

func NewTransport(transport http.RoundTripper) http.RoundTripper {
	if har == nil {
		return transport
	}

	return &harlog.Transport{
		Transport: transport,
		Container: har.Container,
	}
}
