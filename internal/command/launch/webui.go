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
	"github.com/skratchdot/open-golang/open"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/command/mpg"
	"github.com/superfly/flyctl/internal/logger"
	state2 "github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
)

// EditInWebUi launches a web-based editor for the app plan
func (state *launchState) EditInWebUi(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "state.edit_in_web_ui")
	defer span.End()

	session, err := fly.StartCLISession(fmt.Sprintf("%s: %s", state2.Hostname(ctx), state.Plan.AppName), map[string]any{
		"target":   "launch",
		"metadata": state.Plan,
	})
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	if err := open.Run(session.URL); err != nil {
		fmt.Fprintf(io.ErrOut,
			"failed opening browser. Copy the url (%s) into a browser and continue\n",
			session.URL,
		)
	} else {
		colorize := io.ColorScheme()
		fmt.Fprintf(io.Out, "Opening %s ...\n\n", colorize.Bold(session.URL))
	}

	logger := logger.FromContext(ctx)

	finalSession, err := waitForCLISession(ctx, logger, io.ErrOut, session.ID)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("session expired, please try again")
	case err != nil:
		return err
	}

	oldPlan := helpers.Clone(state.Plan)
	state.Plan = &plan.LaunchPlan{}

	// TODO(Ali): Remove me.
	// Hack because somewhere from between UI and here, the numbers get converted to strings
	if err := patchNumbers(finalSession.Metadata, "vm_cpus", "vm_memory"); err != nil {
		return err
	}

	// Wasteful, but gets the job done without uprooting the session types.
	// Just round-trip the map[string]interface{} back into json, so we can re-deserialize it into a complete type.
	metaJson, err := json.Marshal(finalSession.Metadata)
	if err != nil {
		return err
	}
	err = json.Unmarshal(metaJson, &state.Plan)
	if err != nil {
		return err
	}

	// Patch in some fields that we keep in the plan that aren't persisted by the UI.
	// Technically, we should probably just be persisting this, but there's
	// no clear value to the UI having these fields currently.
	if _, ok := finalSession.Metadata["ha"]; !ok {
		state.Plan.HighAvailability = oldPlan.HighAvailability
	}
	// This should never be changed by the UI!!
	state.Plan.ScannerFamily = oldPlan.ScannerFamily

	// Handle database plan from form data
	if pgData, ok := finalSession.Metadata["postgres"].(map[string]interface{}); ok {
		logger.Debugf("Postgres form data: %+v", pgData)
		if mpgData, ok := pgData["managed_postgres"].(map[string]interface{}); ok {
			logger.Debugf("Managed Postgres form data: %+v", mpgData)
			// Validate region for managed Postgres
			region := "iad" // Default region
			if r, ok := mpgData["region"].(string); ok && r != "" {
				region = r
			}

			org, err := state.orgCompact(ctx)
			if err != nil {
				return fmt.Errorf("failed to get organization: %w", err)
			}

			// Check if region is supported for managed Postgres
			validRegion, err := mpg.IsValidMPGRegion(ctx, org.RawSlug, region)
			if err != nil {
				return fmt.Errorf("failed to validate MPG region: %w", err)
			}

			if !validRegion {
				availableCodes, _ := mpg.GetAvailableMPGRegionCodes(ctx, org.Slug)
				return fmt.Errorf("region %s is not available for Managed Postgres. Available regions: %v", region, availableCodes)
			}

			state.Plan.Postgres = plan.PostgresPlan{
				ManagedPostgres: &plan.ManagedPostgresPlan{
					DbName:   state.Plan.AppName + "-db",
					Region:   region,
					Plan:     "basic", // Default plan
					DiskSize: 10,      // Default disk size
				},
			}

			// Apply settings from the form
			// Check both "db_name" (Go struct json tag) and "name" (API/UI convention)
			if dbName, ok := mpgData["db_name"].(string); ok && dbName != "" {
				state.Plan.Postgres.ManagedPostgres.DbName = dbName
			} else if dbName, ok := mpgData["name"].(string); ok && dbName != "" {
				state.Plan.Postgres.ManagedPostgres.DbName = dbName
			}
			if plan, ok := mpgData["plan"].(string); ok && plan != "" {
				state.Plan.Postgres.ManagedPostgres.Plan = plan
			}
			if disk, ok := mpgData["disk"].(float64); ok {
				state.Plan.Postgres.ManagedPostgres.DiskSize = int(disk)
			}
			if clusterID, ok := mpgData["existing_mpg_hashid"].(string); ok && clusterID != "" {
				state.Plan.Postgres.ManagedPostgres.ClusterID = clusterID
			}
		}
	}

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
func waitForCLISession(parent context.Context, logger *logger.Logger, w io.Writer, id string) (session fly.CLISession, err error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = w
	s.Prefix = "Waiting for launch data..."
	s.Start()

	for ctx.Err() == nil {
		if session, err = fly.GetCLISessionState(ctx, id); err != nil {
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
