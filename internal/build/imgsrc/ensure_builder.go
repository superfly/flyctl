package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/haikunator"
	"github.com/superfly/flyctl/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func EnsureBuilder(ctx context.Context, org *fly.Organization, region string, recreateBuilder bool) (*fly.Machine, *fly.App, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "ensure_builder")
	defer span.End()

	if !recreateBuilder {
		builderApp := org.RemoteBuilderApp
		if builderApp != nil {
			span.SetAttributes(attribute.String("builder_app", builderApp.Name))
			flaps, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
				AppName: builderApp.Name,
				// TOOD(billy) make a utility function for App -> AppCompact
				AppCompact: &fly.AppCompact{
					ID:       builderApp.ID,
					Name:     builderApp.Name,
					Status:   builderApp.Status,
					Deployed: builderApp.Deployed,
					Hostname: builderApp.Hostname,
					AppURL:   builderApp.AppURL,
					Organization: &fly.OrganizationBasic{
						ID:       builderApp.Organization.ID,
						Name:     builderApp.Organization.Name,
						Slug:     builderApp.Organization.Slug,
						RawSlug:  builderApp.Organization.RawSlug,
						PaidPlan: builderApp.Organization.PaidPlan,
					},
					PlatformVersion: builderApp.PlatformVersion,
					PostgresAppRole: builderApp.PostgresAppRole,
				},
				OrgSlug: builderApp.Organization.Slug,
			})
			if err != nil {
				tracing.RecordError(span, err, "error creating flaps client")
				return nil, nil, err
			}
			ctx = flapsutil.NewContextWithClient(ctx, flaps)
		}

		builderMachine, err := validateBuilder(ctx, builderApp)
		if err == nil {
			span.AddEvent("builder app already exists and is valid")
			return builderMachine, builderApp, nil
		}

		var validateBuilderErr ValidateBuilderError
		if !errors.As(err, &validateBuilderErr) {
			return nil, nil, err
		}

		if validateBuilderErr == BuilderMachineNotStarted {
			err := restartBuilderMachine(ctx, builderMachine)
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
		}
	} else {
		span.AddEvent("recreating builder")
		if org.RemoteBuilderApp != nil {
			client := flyutil.ClientFromContext(ctx)
			err := client.DeleteApp(ctx, org.RemoteBuilderApp.Name)
			if err != nil {
				tracing.RecordError(span, err, "error deleting existing builder app")
				return nil, nil, err
			}
		}
	}

	builderName := "fly-builder-" + haikunator.Haikunator().Build()
	span.SetAttributes(attribute.String("builder_name", builderName))
	// we want to lauch the machine to the builder
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: builderName,
		OrgSlug: org.Slug,
	})
	if err != nil {
		tracing.RecordError(span, err, "error creating flaps client")
		return nil, nil, err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	app, machine, err := createBuilder(ctx, org, region, builderName)
	if err != nil {
		tracing.RecordError(span, err, "error creating builder")
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
)

func validateBuilder(ctx context.Context, app *fly.App) (*fly.Machine, error) {
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

	if _, err := validateBuilderVolumes(ctx, flapsClient); err != nil {
		tracing.RecordError(span, err, "error validating builder volumes")
		return nil, err
	}
	machine, err := validateBuilderMachines(ctx, flapsClient)
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

func validateBuilderVolumes(ctx context.Context, flapsClient flapsutil.FlapsClient) (*fly.Volume, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "validate_builder_volumes")
	defer span.End()

	var volumes []fly.Volume
	numRetries := 0

	for {
		var err error

		volumes, err = flapsClient.GetVolumes(ctx)
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

func validateBuilderMachines(ctx context.Context, flapsClient flapsutil.FlapsClient) (*fly.Machine, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "validate_builder_machines")
	defer span.End()

	var machines []*fly.Machine
	numRetries := 0
	for {
		var err error

		machines, err = flapsClient.List(ctx, "")
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

func createBuilder(ctx context.Context, org *fly.Organization, region, builderName string) (app *fly.App, mach *fly.Machine, retErr error) {
	ctx, span := tracing.GetTracer().Start(ctx, "create_builder")
	defer span.End()

	client := flyutil.ClientFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	app, retErr = client.CreateApp(ctx, fly.CreateAppInput{
		OrganizationID:  org.ID,
		Name:            builderName,
		AppRoleID:       "remote-docker-builder",
		Machines:        true,
		PreferredRegion: fly.StringPointer(region),
	})
	if retErr != nil {
		tracing.RecordError(span, retErr, "error creating app")
		return nil, nil, retErr
	}

	defer func() {
		if retErr != nil {
			span.AddEvent("cleaning up new builder app due to error")
			client.DeleteApp(ctx, builderName)
		}
	}()

	_, retErr = client.AllocateIPAddress(ctx, app.Name, "shared_v4", "", org, "")
	if retErr != nil {
		tracing.RecordError(span, retErr, "error allocating ip address")
		return nil, nil, retErr
	}

	guest := fly.MachineGuest{
		CPUKind:  "shared",
		CPUs:     4,
		MemoryMB: 4096,
	}
	if org.PaidPlan {
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

	var volume *fly.Volume
	numRetries := 0
	for {
		volume, retErr = flapsClient.CreateVolume(ctx, fly.CreateVolumeRequest{
			Name:                "machine_data",
			SizeGb:              fly.IntPointer(50),
			AutoBackupEnabled:   fly.BoolPointer(false),
			ComputeRequirements: &guest,
			Region:              region,
		})
		if retErr == nil {
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
			flapsClient.DeleteVolume(ctx, volume.ID)
		}
	}()

	mach, retErr = flapsClient.Launch(ctx, fly.LaunchMachineInput{
		Region: region,
		Config: &fly.MachineConfig{
			Env: map[string]string{
				"ALLOW_ORG_SLUG": org.Slug,
				"DATA_DIR":       "/data",
				"LOG_LEVEL":      "debug",
			},
			Guest: &guest,
			Mounts: []fly.MachineMount{
				{
					Path:   "/data",
					Volume: volume.ID,
					Name:   app.Name,
				},
			},
			Services: []fly.MachineService{
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
			},
			Image: lo.Ternary(org.RemoteBuilderImage != "", org.RemoteBuilderImage, "docker-hub-mirror.fly.io/flyio/rchab:sha-9346699"),
		},
	})
	if retErr != nil {
		tracing.RecordError(span, retErr, "error launching builder machine")
		return nil, nil, retErr
	}

	retErr = flapsClient.Wait(ctx, mach, "started", 60*time.Second)
	if retErr != nil {
		tracing.RecordError(span, retErr, "error waiting for builder machine to start")
		return nil, nil, retErr
	}

	return
}

func restartBuilderMachine(ctx context.Context, builderMachine *fly.Machine) error {
	ctx, span := tracing.GetTracer().Start(ctx, "restart_builder_machine")
	defer span.End()

	flapsClient := flapsutil.ClientFromContext(ctx)

	if err := flapsClient.Restart(ctx, fly.RestartMachineInput{
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

	if err := flapsClient.Wait(ctx, builderMachine, "started", time.Second*60); err != nil {
		tracing.RecordError(span, err, "error waiting for builder machine to start")
		return err
	}

	return nil
}
