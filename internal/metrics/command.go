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

	// appID, orgID, appName and loginMethod are guarded by mu.
	appID       string
	orgID       string
	appName     string
	loginMethod string
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
	AppName         string  `json:"app,omitempty"`
	AppID           string  `json:"app_id,omitempty"`
	OrgID           string  `json:"org_id,omitempty"`
	LoginMethod     string  `json:"login_method,omitempty"`
}

// Ways a browser login can reach the CLI, reported by SetLoginMethod.
const (
	LoginMethodCallback     = "callback"      // the browser reached the local callback
	LoginMethodPaste        = "paste"         // the user pasted the code into the terminal
	LoginMethodOtherProcess = "other_process" // another flyctl process (e.g. --code) finished it
	LoginMethodPoll         = "poll"          // a server without PKCE; the CLI polled for the token
)

// SetLoginMethod records how a browser login reached the CLI, to be reported
// with the command's stats.
func SetLoginMethod(method string) {
	mu.Lock()
	defer mu.Unlock()

	loginMethod = method
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

// SetAppName records the name of the app the running command is acting on. The
// name comes from the --app flag, FLY_APP or fly.toml, so it costs nothing to
// resolve and is known to every app-scoped command, including the many that
// never look the app up. Reporting it lets an invocation be attributed to an
// app and an organization after the fact even when our tokens span several
// organizations and ScopedIDs declines to name one.
func SetAppName(name string) {
	mu.Lock()
	defer mu.Unlock()

	if name != "" {
		appName = name
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
			AppName:         appName,
			AppID:           appID,
			OrgID:           orgID,
			LoginMethod:     loginMethod,
		})
	}
}
