package launch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/azazeal/pause"
	"github.com/briandowns/spinner"
	"github.com/google/go-querystring/query"
	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/logger"
	state2 "github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

// EditInWebUi launches a web-based editor for the app plan
func (state *launchState) EditInWebUi(ctx context.Context) error {

	session, err := api.StartCLISession(fmt.Sprintf("%s: %s", state2.Hostname(ctx), state.plan.AppName), map[string]any{
		"target": "launch",
	})
	if err != nil {
		return err
	}

	sessionURL := session.URL

	qsVals, err := query.Values(state.plan)
	if err != nil {
		return fmt.Errorf("error making query string for launch plan: %w", err)
	}
	sessionURL += fmt.Sprintf("?%s", qsVals.Encode())

	io := iostreams.FromContext(ctx)
	if err := open.Run(sessionURL); err != nil {
		fmt.Fprintf(io.ErrOut,
			"failed opening browser. Copy the url (%s) into a browser and continue\n",
			sessionURL,
		)
	}

	logger := logger.FromContext(ctx)

	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "Opening %s ...\n\n", colorize.Bold(sessionURL))

	finalSession, err := waitForCLISession(ctx, logger, io.ErrOut, session.ID)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("session expired, please try again")
	case err != nil:
		return err
	}

	oldPlan := helpers.Clone(state.plan)

	// Hack because somewhere from between UI and here, the numbers get converted to strings
	if err := patchNumbers(finalSession.Metadata, "vm_cpus", "vm_memory"); err != nil {
		return err
	}

	metaJson, err := json.Marshal(finalSession.Metadata)
	if err != nil {
		return err
	}
	err = json.Unmarshal(metaJson, &state.plan)
	if err != nil {
		return err
	}

	state.plan.ScannerFamily = oldPlan.ScannerFamily
	state.plan.cache = oldPlan.cache

	return nil
}

// TODO: I'd like to just fix the round-trip issue here, instead of this bandage.
// This is mostly so I can get a presentation out before I have to leave :)

// patchNumbers is a hack to fix the round-trip issue with numbers being converted to strings
// It supports nested paths, such as "vm_cpus" or "some_struct.int_value"
func patchNumbers(obj map[string]any, labels ...string) error {
outer:
	for _, label := range labels {

		// Borrow down to the right element.
		path := strings.Split(label, ".")
		iface := obj
		var ok bool
		for _, p := range path[:len(path)-1] {
			if iface, ok = iface[p].(map[string]any); ok {
				continue outer
			}
		}

		// Patch the element
		name := path[len(path)-1]
		val, ok := iface[name]
		if !ok {
			continue
		}
		if numStr, ok := val.(string); ok {
			num, err := strconv.ParseInt(numStr, 10, 64)
			if err != nil {
				return err
			}
			iface[name] = num
		}
	}
	return nil
}

// TODO: this does NOT break on interrupts
func waitForCLISession(parent context.Context, logger *logger.Logger, w io.Writer, id string) (session api.CLISession, err error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = w
	s.Prefix = "Waiting for launch data..."
	s.Start()

	for ctx.Err() == nil {
		if session, err = api.GetCLISessionState(ctx, id); err != nil {
			logger.Debugf("failed retrieving token: %v", err)

			pause.For(ctx, time.Second)

			continue
		}

		logger.Debug("retrieved launch data.")

		s.FinalMSG = "Waiting for launch data... Done\n"
		s.Stop()

		break
	}

	return
}
