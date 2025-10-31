package deploy

import (
	"context"
	"fmt"

	"github.com/docker/go-units"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newRemoteDeployment(ctx context.Context, appConfig *appconfig.Config, img *imgsrc.DeploymentImage) error {
	ctx, span := tracing.GetTracer().Start(ctx, "deploy_to_machines_remote")
	defer span.End()

	// TODO(AG): Metrics and tracing.

	appName := appconfig.NameFromContext(ctx)

	apiClient := flyutil.ClientFromContext(ctx)
	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	uiexClient := uiexutil.ClientFromContext(ctx)
	if uiexClient == nil {
		return fmt.Errorf("uiex client not found in context")
	}

	filters := getDeploymentFilters(ctx)
	overrides, err := getDeploymentOverrides(ctx)
	if err != nil {
		return err
	}

	req := uiex.RemoteDeploymentRequest{
		Organization: appCompact.Organization.Slug,
		Config:       appConfig,
		Image:        img.Tag,
		Strategy:     uiex.RemoteDeploymentStrategyRolling,
		BuildId:      img.BuildID,
		BuilderID:    img.BuilderID,
		Filters:      filters,
		Overrides:    overrides,
	}

	streams := iostreams.FromContext(ctx)
	streams.StartProgressIndicator()

	colors := streams.ColorScheme()

	cmdfmt.PrintBegin(streams.ErrOut, "Waiting for the remote deployer")

	response, err := uiexClient.CreateDeploy(ctx, appName, req)
	if err != nil {
		return err
	}

	var sl statuslogger.StatusLogger
	var lineMachineMapping []string

	findLineMachineMapping := func(machineID string) int {
		for i, lineMachineID := range lineMachineMapping {
			if lineMachineID == machineID {
				return i
			}
		}

		// We don't have it.
		lineMachineMapping = append(lineMachineMapping, machineID)
		return len(lineMachineMapping) - 1
	}

	for {
		select {
		case err := <-response.Errors:
			if sl != nil {
				sl.Destroy(false)
			}

			return err

		case <-ctx.Done():
			return ctx.Err()

		case ev, ok := <-response.Events:
			if !ok {
				// Channel is closed, so we're done
				return nil
			}

			switch ev.Type {
			case uiex.DeploymentEventTypeStarted:
				streams.StopProgressIndicator()
				cmdfmt.PrintDone(streams.ErrOut, "Remote deployer ready")

			case uiex.DeploymentEventTypeProgress:
				progress := ev.Data.(*uiex.DeploymentProgress)

				switch progress.Type {
				case uiex.DeploymentProgressTypeInfo:
					info := progress.Data.(uiex.DeploymentProgressInfo)

					var resume statuslogger.ResumeFn
					if sl != nil {
						resume = sl.Pause()
					}

					cmdfmt.PrintDone(streams.ErrOut, info)

					if resume != nil {
						resume()
					}

				case uiex.DeploymentProgressTypePlan:
					plan := progress.Data.(uiex.DeploymentProgressPlan)

					cmdfmt.PrintBegin(streams.ErrOut, "Need to create ", plan.Create, " machines")
					cmdfmt.PrintBegin(streams.ErrOut, "Need to update ", plan.Update, " machines")
					cmdfmt.PrintBegin(streams.ErrOut, "Need to destroy ", plan.Delete, " machines")

					sl = statuslogger.Create(ctx, plan.Create+plan.Update+plan.Delete, true)

				case uiex.DeploymentProgressTypeUpdate:
					update := progress.Data.(uiex.DeploymentProgressUpdate)
					machineID := update.MachineID
					// TODO(AG): Check if machine ID is empty. Error if it is.

					mapping := findLineMachineMapping(machineID)
					line := sl.Line(mapping)
					writeMachineUpdate(line, update, colors)

				default:
					cmdfmt.PrintDone(streams.ErrOut, "Unknown progress event ", progress.Type)
				}

			case uiex.DeploymentEventTypeSuccess:
				if sl != nil {
					sl.Destroy(false)
				}
				cmdfmt.PrintDone(streams.ErrOut, colors.GreenBold("Deployment completed"))

			case uiex.DeploymentEventTypeError:
				var resume statuslogger.ResumeFn
				if sl != nil {
					resume = sl.Pause()
				}

				cmdfmt.PrintDone(streams.ErrOut,
					colors.RedBold("Deployment Error "), ev.Data.(uiex.DeploymentEventError))

				if resume != nil {
					resume()
				}

			default:
				cmdfmt.PrintDone(streams.ErrOut, "Unknown event ", ev.Type)
			}
		}
	}
}

func writeMachineUpdate(line statuslogger.StatusLine, update uiex.DeploymentProgressUpdate, colors *iostreams.ColorScheme) {
	switch update.Type {
	case "starting_update":
		line.LogfStatus(statuslogger.StatusNone, "[%s] Preparing to update", update.MachineID)

	case "acquiring_lease":
		line.LogfStatus(statuslogger.StatusRunning, "[%s] Acquiring lease", update.MachineID)

	case "lease_acquired":
		line.LogfStatus(statuslogger.StatusRunning, "[%s] Updating", update.MachineID)

	case "created":
		line.LogfStatus(statuslogger.StatusRunning,
			"[%s] Machine created for process group %q", update.MachineID, update.ProcessGroup)

	case "starting_remove":
		line.LogfStatus(statuslogger.StatusNone,
			"[%s] Destroying machine for process group %q", update.MachineID, update.ProcessGroup)

	case "removed":
		line.LogfStatus(statuslogger.StatusSuccess,
			"[%s] Machine destroyed for process group %q", update.MachineID, update.ProcessGroup)

	case "updated":
		line.LogfStatus(statuslogger.StatusRunning, "[%s] Releasing lease", update.MachineID)

	case "lease_released":
		line.LogfStatus(statuslogger.StatusRunning, "[%s] Lease released", update.MachineID)

	case "waiting_for_started":
		line.LogfStatus(statuslogger.StatusRunning,
			"[%s] Waiting for machine to start (current state: %s)", update.MachineID, update.State)

	case "started":
		line.LogfStatus(statuslogger.StatusRunning, "[%s] Machine started", update.MachineID)

	case "waiting_for_healthy":
		line.LogfStatus(statuslogger.StatusRunning,
			"[%s] Waiting for health checks (%d/%d)", update.MachineID, update.PassingChecks, update.TotalChecks)

	case "healthy":
		line.LogfStatus(statuslogger.StatusSuccess,
			"[%s] %s", update.MachineID, colors.Green("Done"))
	}
}

func getDeploymentFilters(ctx context.Context) *uiex.RemoteDeploymentFilters {
	excludeRegions := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "exclude-regions") {
		excludeRegions[r] = true
	}

	onlyRegions := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "regions") {
		onlyRegions[r] = true
	}

	excludeMachines := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "exclude-machines") {
		excludeMachines[r] = true
	}

	onlyMachines := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "only-machines") {
		onlyMachines[r] = true
	}

	processGroups := make(map[string]bool)
	for _, r := range flag.GetNonEmptyStringSlice(ctx, "process-groups") {
		processGroups[r] = true
	}

	setToSlice := func(m map[string]bool) []string {
		s := make([]string, 0, len(m))
		for k := range m {
			s = append(s, k)
		}
		return s
	}

	return &uiex.RemoteDeploymentFilters{
		ExcludeRegions:  setToSlice(excludeRegions),
		Regions:         setToSlice(onlyRegions),
		ExcludeMachines: setToSlice(excludeMachines),
		OnlyMachines:    setToSlice(onlyMachines),
		ProcessGroups:   setToSlice(processGroups),
	}
}

func getDeploymentOverrides(ctx context.Context) (*uiex.RemoteDeploymentOverrides, error) {
	primaryRegion := flag.GetString(ctx, "primary-region")
	cpuKind := flag.GetString(ctx, "vm-cpu-kind")
	vmSize := flag.GetString(ctx, "vm-size")

	cpus := 0
	if flag.IsSpecified(ctx, "vm-cpus") {
		cpus = flag.GetInt(ctx, "vm-cpus")
		if cpus <= 0 {
			return nil, fmt.Errorf("--vm-cpus must be greater than zero, got: %d", cpus)
		}
	}

	memory := 0
	if flag.IsSpecified(ctx, "vm-memory") {
		rawValue := flag.GetString(ctx, "vm-memory")
		memoryMB, err := helpers.ParseSize(rawValue, units.RAMInBytes, units.MiB)
		switch {
		case err != nil:
			return nil, err
		case memoryMB == 0:
			return nil, fmt.Errorf("--vm-memory cannot be zero")
		default:
			memory = memoryMB
		}
	}

	return &uiex.RemoteDeploymentOverrides{
		PrimaryRegion: primaryRegion,
		VmCPUs:        cpus,
		VmMemory:      memory,
		VmCPUKind:     cpuKind,
		VmSize:        vmSize,
	}, nil
}
