package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/client-signals/go"
	"github.com/superfly/flyctl/internal/instrument"
)

var (
	processStartTime = time.Now()
	commandContext   context.Context
	mu               sync.Mutex

	// appID and orgID are guarded by mu.
	appID string
	orgID string
)

var IsUsingGPU bool

type commandStats struct {
	Command         string  `json:"c"`
	Duration        float64 `json:"d"`
	GraphQLCalls    int     `json:"gc"`
	GraphQLDuration float64 `json:"gd"`
	FlapsCalls      int     `json:"fc"`
	FlapsDuration   float64 `json:"fd"`
	UsingGPU        bool    `json:"gpu"`
	Failed          bool    `json:"f"`
	Operator        string  `json:"op,omitempty"`
	AgentName       string  `json:"ag,omitempty"`
	AppID           string  `json:"app_id,omitempty"`
	OrgID           string  `json:"org_id,omitempty"`
}

// SetAppOrgIDs records the internal numeric IDs of the app and organization the
// running command is acting on, to be reported with its stats. Either argument
// may be empty, so a caller that only knows one of the two can leave the other
// alone.
func SetAppOrgIDs(app, org string) {
	mu.Lock()
	defer mu.Unlock()

	if app != "" {
		appID = app
	}

	if org != "" {
		orgID = org
	}
}

func RecordCommandContext(ctx context.Context) {
	mu.Lock()
	defer mu.Unlock()

	if commandContext != nil {
		panic("called metrics.RecordCommandContext twice")
	}

	commandContext = ctx
}

func RecordCommandFinish(cmd *cobra.Command, failed bool) {
	mu.Lock()
	defer mu.Unlock()

	duration := time.Since(processStartTime)

	graphql := instrument.GraphQL.Get()
	flaps := instrument.Flaps.Get()

	if commandContext != nil {
		operator, agentName := OperatorFromSignals(clientsignals.DetectOnce())

		Send(commandContext, "command/stats", commandStats{
			Command:         cmd.CommandPath(),
			Duration:        duration.Seconds(),
			GraphQLCalls:    graphql.Calls,
			GraphQLDuration: graphql.Duration,
			FlapsCalls:      flaps.Calls,
			FlapsDuration:   flaps.Duration,
			UsingGPU:        IsUsingGPU,
			Failed:          failed,
			Operator:        operator,
			AgentName:       agentName,
			AppID:           appID,
			OrgID:           orgID,
		})
	}
}
