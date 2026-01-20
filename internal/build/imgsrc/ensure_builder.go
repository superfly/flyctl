package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/haikunator"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Provisioner struct {
	orgID                 string
	orgSlug               string
	orgTrial              bool
	orgPaidPlan           bool
	orgRemoteBuilderImage string
	useVolume             bool
	buildkitAddr          string
	buildkitImage         string
}

func NewProvisionerUiexOrg(org *uiex.Organization) *Provisioner {
	return &Provisioner{
		orgID:                 org.ID,
		orgSlug:               org.Slug,
		orgTrial:              org.BillingStatus == uiex.BillingStatusTrialActive,
		orgPaidPlan:           org.PaidPlan,
		orgRemoteBuilderImage: org.RemoteBuilderImage,
		useVolume:             true,
	}
}

func NewBuildkitProvisioner(org *uiex.Organization, addr, image string) *Provisioner {
	return &Provisioner{
		orgID:                 org.ID,
		orgSlug:               org.Slug,
		orgPaidPlan:           org.PaidPlan,
		orgRemoteBuilderImage: org.RemoteBuilderImage,
		useVolume:             true,
		buildkitAddr:          addr,
		buildkitImage:         image,
	}
}

func (p *Provisioner) UseBuildkit() bool {
	return p.buildkitAddr != "" || p.buildkitImage != ""
}

const defaultImage = "docker-hub-mirror.fly.io/flyio/rchab:sha-9346699"
const DefaultBuildkitImage = "docker-hub-mirror.fly.io/flyio/buildkit@sha256:0fe49e6f506f0961cb2fc45d56171df0e852229facf352f834090345658b7e1c"
const appRoleRemoteBuilder = "remote-docker-builder"

func (p *Provisioner) image() string {
	if p.buildkitImage != "" {
		return p.buildkitImage
	}
	if p.orgRemoteBuilderImage != "" {
		return p.orgRemoteBuilderImage
	}
	return defaultImage
}

// GetRemoteBuilderApp returns the org's first app with appRoleRemoteBuilder, or nil if it none exist
func (p *Provisioner) GetRemoteBuilderApp(ctx context.Context) (*flaps.App, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)

	apps, err := flapsClient.ListApps(ctx, flaps.ListAppsRequest{
		OrgSlug: p.orgSlug,
		AppRole: appRoleRemoteBuilder,
	})
	if err != nil {
		return nil, err
	}

	if len(apps) == 0 {
		return nil, nil
	}

	return &apps[0], nil
}

func (p *Provisioner) EnsureBuilder(ctx context.Context, region string, recreateBuilder bool) (*fly.Machine, *flaps.App, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "ensure_builder")
	defer span.End()

	builderApp, err := p.GetRemoteBuilderApp(ctx)
	if err != nil {
		return nil, nil, err
	}

	if !recreateBuilder {
		if builderApp != nil {
			span.SetAttributes(attribute.String("builder_app", builderApp.Name))
		}

		builderMachine, err := p.validateBuilder(ctx, builderApp)
		if err == nil {
			span.AddEvent("builder app already exists and is valid")
			return builderMachine, builderApp, nil
		}

		var validateBuilderErr ValidateBuilderError
		if !errors.As(err, &validateBuilderErr) {
			return nil, nil, err
		}

		if validateBuilderErr == BuilderMachineNotStarted {
			err := restartBuilderMachine(ctx, builderApp.Name, builderMachine)
			switch {
			case errors.Is(err, ShouldReplaceBuilderMachine):
				span.AddEvent("recreating builder due to resource reservation error")
			case err != nil:
				tracing.RecordError(span, err, "error restarting builder machine")
				return nil, nil, err
			default:
				return builderMachine, builderApp, nil

			}
		}

		if validateBuilderErr != NoBuilderApp {
			span.AddEvent(fmt.Sprintf("deleting existing invalid builder due to %s", validateBuilderErr))
			client := flyutil.ClientFromContext(ctx)
			err := client.DeleteApp(ctx, builderApp.Name)
			if err != nil {
				tracing.RecordError(span, err, "error deleting invalid builder app")
				return nil, nil, err
			}

			_ = appsecrets.DeleteMinvers(ctx, builderApp.Name)
		}
	} else {
		span.AddEvent("recreating builder")
		if builderApp != nil {
			client := flyutil.ClientFromContext(ctx)
			err := client.DeleteApp(ctx, builderApp.Name)
			if err != nil {
				tracing.RecordError(span, err, "error deleting existing builder app")
				return nil, nil, err
			}

			_ = appsecrets.DeleteMinvers(ctx, builderApp.Name)
		}
	}

	builderName := "fly-builder-" + haikunator.Haikunator().Build()
	span.SetAttributes(attribute.String("builder_name", builderName))
	// we want to lauch the machine to the builder
	app, machine, err := p.createBuilder(ctx, region, builderName)
	if err != nil {
		tracing.RecordError(span, err, "error creating builder")
		return nil, nil, err
	}
	return machine, app, nil
}

func EnsureFlyManagedBuilder(ctx context.Context, orgSlug string, region string) (*fly.Machine, *flaps.App, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "ensure_fly_managed_builder")
	defer span.End()

	app, machine, err := createFlyManagedBuilder(ctx, orgSlug, region)
	if err != nil {
		tracing.RecordError(span, err, "error creating fly managed builder")
		return nil, nil, err
	}
	return machine, app, nil
}

type ValidateBuilderError int

func (e ValidateBuilderError) Error() string {
	switch e {
	case NoBuilderApp:
		return "no builder app"
	case NoBuilderVolume:
		return "no builder volume"
	case InvalidMachineCount:
		return "invalid machine count"
	case BuilderMachineNotStarted:
		return "builder machine not started"
	case ShouldReplaceBuilderMachine:
		return "should replace builder machine"
	default:
		return "unknown error validating builder"
	}
}

const (
	NoBuilderApp ValidateBuilderError = iota
	NoBuilderVolume
	InvalidMachineCount
	BuilderMachineNotStarted
	ShouldReplaceBuilderMachine

	buildkitGRPCPort = 1234
)

// validateBuilder returns a machine if it is available for building images.
func (p *Provisioner) validateBuilder(ctx context.Context, app *flaps.App) (*fly.Machine, error) {
	machine, err := p.validateBuilderMachine(ctx, app)
	if err != nil {
		// validateBuilderMachine returns a machine even if there is an error.
		return machine, err
	}

	// Don't run extra checks for non-Buildkit cases.
	if !p.UseBuildkit() {
		return machine, nil
	}

	// If not, make sure the machine is configured for Buildkit.
	if len(machine.Config.Services) == 1 && machine.Config.Services[0].InternalPort == buildkitGRPCPort {
		return machine, nil
	}
	return nil, ShouldReplaceBuilderMachine
}

func (p *Provisioner) validateBuilderMachine(ctx context.Context, app *flaps.App) (*fly.Machine, error) {
	var builderAppName string
	if app != nil {
		builderAppName = app.Name
	}
	ctx, span := tracing.GetTracer().Start(ctx, "validate_builder", trace.WithAttributes(attribute.String("builder_app", builderAppName)))
	defer span.End()

	if app == nil {
		tracing.RecordError(span, NoBuilderApp, "no builder app")
		return nil, NoBuilderApp
	}

	flapsClient := flapsutil.ClientFromContext(ctx)

	if p.useVolume {
		if _, err := validateBuilderVolumes(ctx, flapsClient, app.Name); err != nil {
			tracing.RecordError(span, err, "error validating builder volumes")
			return nil, err
		}
	}
	machine, err := validateBuilderMachines(ctx, flapsClient, app.Name)
	if err != nil {
		tracing.RecordError(span, err, "error validating builder machines")
		return nil, err
	}

	if machine.State != "started" {
		tracing.RecordError(span, BuilderMachineNotStarted, "builder machine not started")
		return machine, BuilderMachineNotStarted
	}

	return machine, nil
}

func validateBuilderVolumes(ctx context.Context, flapsClient flapsutil.FlapsClient, appName string) (*fly.Volume, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "validate_builder_volumes")
	defer span.End()

	var volumes []fly.Volume
	numRetries := 0

	for {
		var err error

		volumes, err = flapsClient.GetVolumes(ctx, appName)
		if err == nil {
			break
		}

		var flapsErr *flaps.FlapsError
		// if it isn't a server error, no point in retrying
		if errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode >= 500 && flapsErr.ResponseStatusCode < 600 {
			span.AddEvent(fmt.Sprintf("non-server error %d", flapsErr.ResponseStatusCode))
			numRetries += 1

			if numRetries >= 3 {
				tracing.RecordError(span, err, "error getting volumes")
				return nil, err
			}
			time.Sleep(1 * time.Second)
		} else {
			tracing.RecordError(span, err, "error getting volumes")
			return nil, err
		}
	}

	if len(volumes) == 0 {
		tracing.RecordError(span, NoBuilderVolume, "the existing builder app has no volume")
		return nil, NoBuilderVolume
	}

	return &volumes[0], nil
}

func validateBuilderMachines(ctx context.Context, flapsClient flapsutil.FlapsClient, appName string) (*fly.Machine, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "validate_builder_machines")
	defer span.End()

	var machines []*fly.Machine
	numRetries := 0
	for {
		var err error

		machines, err = flapsClient.List(ctx, appName, "")
		if err == nil {
			break
		}

		var flapsErr *flaps.FlapsError
		// if it isn't a server error, no point in retrying
		if errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode >= 500 && flapsErr.ResponseStatusCode < 600 {
			span.AddEvent(fmt.Sprintf("non-server error %d", flapsErr.ResponseStatusCode))
			numRetries += 1

			if numRetries >= 3 {
				tracing.RecordError(span, err, "error listing machines")
				return nil, err
			}
			time.Sleep(1 * time.Second)
		} else {
			tracing.RecordError(span, err, "error listing machines")
			return nil, err
		}
	}

	if len(machines) != 1 {
		span.AddEvent(fmt.Sprintf("invalid machine count %d", len(machines)))
		tracing.RecordError(span, InvalidMachineCount, "the existing builder app has an invalid number of machines")
		return nil, InvalidMachineCount
	}

	return machines[0], nil
}

func (p *Provisioner) createBuilder(ctx context.Context, region, builderName string) (app *flaps.App, mach *fly.Machine, retErr error) {
	buildkit := p.UseBuildkit()

	ctx, span := tracing.GetTracer().Start(ctx, "create_builder")
	defer span.End()

	client := flyutil.ClientFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	app, retErr = flapsClient.CreateApp(ctx, flaps.CreateAppRequest{
		Org:       p.orgSlug,
		Name:      builderName,
		AppRoleID: appRoleRemoteBuilder,
	})
	if retErr != nil {
		tracing.RecordError(span, retErr, "error creating app")
		return nil, nil, retErr
	}

	defer func() {
		if retErr != nil {
			span.AddEvent("cleaning up new builder app due to error")
			client.DeleteApp(ctx, builderName)
			_ = appsecrets.DeleteMinvers(ctx, builderName)
		}
	}()

	if buildkit {
		_, retErr = client.AllocateIPAddress(ctx, app.Name, "private_v6", "", p.orgID, "")
		if retErr != nil {
			tracing.RecordError(span, retErr, "error allocating ip address")
			return nil, nil, retErr
		}
	} else {
		_, retErr = client.AllocateIPAddress(ctx, app.Name, "shared_v4", "", p.orgID, "")
		if retErr != nil {
			tracing.RecordError(span, retErr, "error allocating ip address")
			return nil, nil, retErr
		}
	}

	guest := fly.MachineGuest{
		CPUKind:  "shared",
		CPUs:     4,
		MemoryMB: 4096,
	}
	if p.orgPaidPlan && !p.orgTrial {
		guest = fly.MachineGuest{
			CPUKind:  "shared",
			CPUs:     8,
			MemoryMB: 8192,
		}
	}

	retErr = flapsClient.WaitForApp(ctx, app.Name)
	if retErr != nil {
		tracing.RecordError(span, retErr, "error waiting for builder")
		return nil, nil, fmt.Errorf("waiting for app %s: %w", app.Name, retErr)
	}

	config := &fly.MachineConfig{
		Env: map[string]string{
			"ALLOW_ORG_SLUG": p.orgSlug,
			"LOG_LEVEL":      "debug",
		},
		Guest: &guest,
		Image: p.image(),
	}

	if buildkit {
		config.Services = []fly.MachineService{
			{
				InternalPort: 1234,
				Ports:        []fly.MachinePort{{Port: fly.IntPointer(1234)}},
				Autostart:    fly.BoolPointer(true),
				Autostop:     fly.Pointer(fly.MachineAutostopStop),
			},
		}
	} else {
		config.Services = []fly.MachineService{
			{
				Protocol:           "tcp",
				InternalPort:       8080,
				Autostop:           fly.Pointer(fly.MachineAutostopOff),
				Autostart:          fly.BoolPointer(true),
				MinMachinesRunning: fly.IntPointer(0),
				Ports: []fly.MachinePort{
					{
						Port:       fly.IntPointer(80),
						Handlers:   []string{"http"},
						ForceHTTPS: true,
						HTTPOptions: &fly.HTTPOptions{
							H2Backend: fly.BoolPointer(true),
						},
					},
					{
						Port:     fly.IntPointer(443),
						Handlers: []string{"http", "tls"},
						TLSOptions: &fly.TLSOptions{
							ALPN: []string{"h2"},
						},
						HTTPOptions: &fly.HTTPOptions{
							H2Backend: fly.BoolPointer(true),
						},
					},
				},
				ForceInstanceKey: nil,
			},
		}
	}

	if p.useVolume {
		var volume *fly.Volume
		numRetries := 0
		for {
			volume, retErr = flapsClient.CreateVolume(ctx, builderName, fly.CreateVolumeRequest{
				Name:                "machine_data",
				SizeGb:              fly.IntPointer(50),
				AutoBackupEnabled:   fly.BoolPointer(false),
				ComputeRequirements: &guest,
				Region:              region,
			})
			if retErr == nil {
				region = volume.Region
				break
			}

			var flapsErr *flaps.FlapsError
			if errors.As(retErr, &flapsErr) && flapsErr.ResponseStatusCode >= 500 && flapsErr.ResponseStatusCode < 600 {
				span.AddEvent(fmt.Sprintf("non-server error %d", flapsErr.ResponseStatusCode))
				numRetries += 1

				if numRetries >= 5 {
					tracing.RecordError(span, retErr, "error creating volume")
					return nil, nil, retErr
				}
				time.Sleep(1 * time.Second)
			} else {
				tracing.RecordError(span, retErr, "error creating volume")
				return nil, nil, retErr
			}
		}

		defer func() {
			if retErr != nil {
				span.AddEvent("cleaning up new volume due to error")
				flapsClient.DeleteVolume(ctx, builderName, volume.ID)
			}
		}()

		if buildkit {
			config.Mounts = append(config.Mounts, fly.MachineMount{
				Path:   "/var/lib/buildkit",
				Volume: volume.ID,
				Name:   app.Name,
			})
		} else {
			config.Env["DATA_DIR"] = "/data"
			config.Mounts = append(config.Mounts, fly.MachineMount{
				Path:   "/data",
				Volume: volume.ID,
				Name:   app.Name,
			})
		}
	}

	minvers, err := appsecrets.GetMinvers(app.Name)
	if err != nil {
		return nil, nil, err
	}
	mach, retErr = flapsClient.Launch(ctx, builderName, fly.LaunchMachineInput{
		Region:            region,
		Config:            config,
		MinSecretsVersion: minvers,
	})
	if retErr != nil {
		tracing.RecordError(span, retErr, "error launching builder machine")
		return nil, nil, retErr
	}

	retErr = flapsClient.Wait(ctx, builderName, mach, "started", 180*time.Second) // 3 minutes for machine start + DNS propagation
	if retErr != nil {
		tracing.RecordError(span, retErr, "error waiting for builder machine to start")
		return nil, nil, retErr
	}

	return
}

func createFlyManagedBuilder(ctx context.Context, orgSlug string, region string) (app *flaps.App, mach *fly.Machine, retErr error) {
	ctx, span := tracing.GetTracer().Start(ctx, "create_builder")
	defer span.End()

	uiexClient := uiexutil.ClientFromContext(ctx)

	response, error := uiexClient.CreateFlyManagedBuilder(ctx, orgSlug, region)
	if error != nil {
		tracing.RecordError(span, retErr, "error creating managed builder")
		return nil, nil, retErr
	}

	builderApp := &flaps.App{
		Name: response.Data.AppName,
	}

	machine := &fly.Machine{
		ID: response.Data.MachineID,
	}

	return builderApp, machine, nil
}

func restartBuilderMachine(ctx context.Context, appName string, builderMachine *fly.Machine) error {
	ctx, span := tracing.GetTracer().Start(ctx, "restart_builder_machine")
	defer span.End()

	flapsClient := flapsutil.ClientFromContext(ctx)

	if err := flapsClient.Restart(ctx, appName, fly.RestartMachineInput{
		ID: builderMachine.ID,
	}, ""); err != nil {
		if strings.Contains(err.Error(), "could not reserve resource for machine") ||
			strings.Contains(err.Error(), "deploys to this host are temporarily disabled") {
			span.RecordError(err)
			return ShouldReplaceBuilderMachine
		}

		tracing.RecordError(span, err, "error restarting builder machine")
		return err
	}

	if err := flapsClient.Wait(ctx, appName, builderMachine, "started", time.Second*180); err != nil { // 3 minutes for restart + DNS propagation
		tracing.RecordError(span, err, "error waiting for builder machine to start")
		return err
	}

	return nil
}
