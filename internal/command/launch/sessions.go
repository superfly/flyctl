package launch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
	state2 "github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func newSessions() *cobra.Command {
	sessDesc := "manage launch sessions"
	cmd := command.New("sessions", sessDesc, sessDesc, nil)
	// not that useful anywhere else yet
	cmd.Hidden = true

	createDesc := "create a new launch session"
	createCmd := command.New("create", createDesc, createDesc, runSessionCreate, command.LoadAppConfigIfPresent)

	flag.Add(createCmd,
		flag.App(),
		flag.Region(),
		flag.Org(),
		flag.AppConfig(),
		flag.String{
			Name:        "name",
			Description: `Name of the new app`,
		},
		// don't try to generate a name
		flag.Bool{
			Name:        "force-name",
			Description: "Force app name supplied by --name",
			Default:     false,
			Hidden:      true,
		},
		flag.Int{
			Name:        "internal-port",
			Description: "Set internal_port for all services in the generated fly.toml",
			Default:     -1,
		},
		flag.Bool{
			Name:        "ha",
			Description: "Create spare machines that increases app availability",
			Default:     false,
		},
		flag.String{
			Name:        "session-path",
			Description: "Path to write the session info to",
			Default:     "session.json",
		},
		flag.String{
			Name:        "manifest-path",
			Description: "Path to write the manifest info to",
			Default:     "manifest.json",
		},
		flag.Bool{
			Name:        "copy-config",
			Description: "Use the configuration file if present without prompting",
			Default:     false,
		},
		flag.String{
			Name: "from-manifest",
		},
	)

	// not that useful anywhere else yet
	createCmd.Hidden = true

	finalizeDesc := "finalize a launch session"
	finalizeCmd := command.New("finalize", finalizeDesc, finalizeDesc, runSessionFinalize, command.LoadAppConfigIfPresent)

	flag.Add(finalizeCmd,
		flag.App(),
		flag.Region(),
		flag.Org(),
		flag.AppConfig(),
		flag.String{
			Name:        "session-path",
			Description: "Path to write the session info to",
			Default:     "session.json",
		},
		flag.String{
			Name:        "manifest-path",
			Description: "Path to write the manifest info to",
			Default:     "manifest.json",
		},
		flag.String{
			Name:        "from-file",
			Description: "Path to a CLI session JSON file",
			Default:     "",
		},
	)

	// not that useful anywhere else yet
	finalizeCmd.Hidden = true

	cmd.AddCommand(createCmd, finalizeCmd)

	return cmd
}

func runSessionCreate(ctx context.Context) (err error) {
	var (
		launchManifest *LaunchManifest
		cache          *planBuildCache
	)

	launchManifest, err = getManifestArgument(ctx)
	if err != nil {
		return err
	}

	if launchManifest != nil {
		// we loaded a manifest...
		cache = &planBuildCache{
			appConfig:        launchManifest.Config,
			sourceInfo:       nil,
			appNameValidated: true,
			warnedNoCcHa:     true,
		}
	}

	// recoverableErrors := recoverableErrorBuilder{canEnterUi: false}
	// launchManifest, planBuildCache, err := buildManifest(ctx, nil, &recoverableErrors)
	// if err != nil {
	// 	return err
	// }

	// updateConfig(launchManifest.Plan, nil, launchManifest.Config)
	// if n := flag.GetInt(ctx, "internal-port"); n > 0 {
	// 	launchManifest.Config.SetInternalPort(n)
	// }

	manifestPath := flag.GetString(ctx, "manifest-path")

	file, err := os.Create(manifestPath)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonEncoder := json.NewEncoder(file)
	jsonEncoder.SetIndent("", "  ")

	if err := jsonEncoder.Encode(launchManifest); err != nil {
		return err
	}

	file.Close()

	state := &launchState{
		workingDir:     ".",
		configPath:     "fly.json",
		LaunchManifest: *launchManifest,
		env:            map[string]string{},
		planBuildCache: *cache,
		cache:          map[string]interface{}{},
	}

	session, err := fly.StartCLISession(fmt.Sprintf("%s: %s", state2.Hostname(ctx), state.Plan.AppName), map[string]any{
		"target":   "launch",
		"metadata": state.Plan,
	})
	if err != nil {
		return err
	}

	sessionPath := flag.GetString(ctx, "session-path")

	file, err = os.Create(sessionPath)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonEncoder = json.NewEncoder(file)
	jsonEncoder.SetIndent("", "  ")

	if err := jsonEncoder.Encode(session); err != nil {
		return err
	}

	return nil
}

func runSessionFinalize(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	logger := logger.FromContext(ctx)

	var finalMeta map[string]interface{}

	if customizePath := flag.GetString(ctx, "from-file"); customizePath != "" {
		sessionBytes, err := os.ReadFile(customizePath)
		if err != nil {
			return err
		}

		if err := json.Unmarshal(sessionBytes, &finalMeta); err != nil {
			return err
		}
	} else {
		sessionBytes, err := os.ReadFile(flag.GetString(ctx, "session-path"))
		if err != nil {
			return err
		}

		var session fly.CLISession
		if err := json.Unmarshal(sessionBytes, &session); err != nil {
			return err
		}

		// FIXME: better timeout here
		ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()

		finalSession, err := waitForCLISession(ctx, logger, io.ErrOut, session.ID)
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return errors.New("session expired, please try again")
		case err != nil:
			return err
		}

		finalMeta = finalSession.Metadata
	}

	fmt.Printf("final meta is %+v\n", finalMeta)

	manifestBytes, err := os.ReadFile(flag.GetString(ctx, "manifest-path"))
	if err != nil {
		return err
	}

	var launchManifest LaunchManifest
	if err := json.Unmarshal(manifestBytes, &launchManifest); err != nil {
		return err
	}

	planBuildCache := planBuildCache{
		appConfig:        launchManifest.Config,
		sourceInfo:       nil,
		appNameValidated: true,
		warnedNoCcHa:     true,
	}

	// Hack because somewhere from between UI and here, the numbers get converted to strings
	if err := patchNumbers(finalMeta, "vm_cpus", "vm_memory"); err != nil {
		return err
	}

	state := &launchState{
		workingDir:     ".",
		configPath:     "fly.json",
		LaunchManifest: launchManifest,
		env:            map[string]string{},
		planBuildCache: planBuildCache,
		cache:          map[string]interface{}{},
	}

	oldPlan := helpers.Clone(state.Plan)

	// Wasteful, but gets the job done without uprooting the session types.
	// Just round-trip the map[string]interface{} back into json, so we can re-deserialize it into a complete type.
	metaJson, err := json.Marshal(finalMeta)
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
	if _, ok := finalMeta["ha"]; !ok {
		state.Plan.HighAvailability = oldPlan.HighAvailability
	}
	// This should never be changed by the UI!!
	state.Plan.ScannerFamily = oldPlan.ScannerFamily

	fmt.Printf("final state is %+v\n", state)

	updateConfig(state.Plan, nil, state.Config)

	manifestPath := flag.GetString(ctx, "manifest-path")

	file, err := os.Create(manifestPath)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonEncoder := json.NewEncoder(file)
	jsonEncoder.SetIndent("", "  ")

	if err := jsonEncoder.Encode(state.LaunchManifest); err != nil {
		return err
	}

	file.Close()

	return nil
}
