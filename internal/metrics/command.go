package metrics

import (
	"context"
	"github.com/spf13/cobra"
	"time"
)

var (
	processStartTime = time.Now()
	commandContext   context.Context
)

type commandTimingData struct {
	Duration float64 `json:"duration_seconds"`
	Command  string  `json:"command"`
}

func RecordCommandContext(ctx context.Context) {
	if commandContext != nil {
		panic("called metrics.RecordCommandContext twice")
	}

	commandContext = ctx
}

func RecordCommandFinish(cmd *cobra.Command) {
	duration := time.Since(processStartTime)

	data := commandTimingData{
		Duration: duration.Seconds(),
		Command:  cmd.CommandPath(),
	}

	if commandContext != nil {
		Send(commandContext, "command/duration", data)
	}
}
