package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/instrument"
)

var (
	processStartTime = time.Now()
	commandContext   context.Context
	mu               sync.Mutex
)

type commandStats struct {
	Command         string  `json:"c"`
	Duration        float64 `json:"d"`
	GraphQLCalls    int     `json:"gc"`
	GraphQLDuration float64 `json:"gd"`
	FlapsCalls      int     `json:"fc"`
	FlapsDuration   float64 `json:"fd"`
	UsingGPU        bool    `json:"gpu"`
	Failed          bool    `json:"f"`
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
	usingGPU := isUsingGPU()

	if commandContext != nil {
		Send(commandContext, "command/stats", commandStats{
			Command:         cmd.CommandPath(),
			Duration:        duration.Seconds(),
			GraphQLCalls:    graphql.Calls,
			GraphQLDuration: graphql.Duration,
			FlapsCalls:      flaps.Calls,
			FlapsDuration:   flaps.Duration,
			UsingGPU:        usingGPU,
			Failed:          failed,
		})
	}
}

// I hate this, but I want to make sure that if we load the app config, I can grab it in metrisc
var AppConfig *appconfig.Config = nil

func isUsingGPU() bool {
	if AppConfig != nil {
		return lo.SomeBy(AppConfig.Compute, func(x *appconfig.Compute) bool {
			return x != nil && x.MachineGuest != nil && x.MachineGuest.GPUKind != ""
		})
	}
	return false
}
