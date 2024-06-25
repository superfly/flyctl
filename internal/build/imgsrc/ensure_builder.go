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
)

func EnsureBuilder(ctx context.Context, org *fly.Organization, region string, recreateBuilder bool) (*fly.Machine, *fly.App, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "ensure_builder")
	defer span.End()

	if !recreateBuilder {
		builderApp := org.RemoteBuilderApp
		if builderApp != nil {
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
			span.AddEvent("builder machine not started, restarting")
			flapsClient := flapsutil.ClientFromContext(ctx)

			_, err := retryFlapsCall(ctx, 5, func() (*fly.Machine, error) {
				err := flapsClient.Restart(ctx, fly.RestartMachineInput{
					ID: builderMachine.ID,
				}, "")
				return nil, err
			})

			if err != nil {
				if strings.Contains(err.Error(), "machine still restarting") {
					// if the builder machine is starting, it's fine
					return builderMachine, builderApp, nil
				} else {
					tracing.RecordError(span, err, "error restarting builder machine")
					return nil, nil, err
				}
			}

			return builderMachine, builderApp, nil
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
	default:
		return "unknown error validating builder"
	}
}

const (
	NoBuilderApp ValidateBuilderError = iota
	NoBuilderVolume
	InvalidMachineCount
	BuilderMachineNotStarted
)

func validateBuilder(ctx context.Context, app *fly.App) (*fly.Machine, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "validate_builder")
	defer span.End()

	if app == nil {
		tracing.RecordError(span, NoBuilderApp, "no builder app")
		return nil, NoBuilderApp
	}

	span.AddEvent(fmt.Sprintf("validating builder %s", app.Name))

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

	volumes, err := retryFlapsCall(ctx, 5, func() ([]fly.Volume, error) {
		return flapsClient.GetVolumes(ctx)
	})
	if err != nil {
		if strings.Contains(err.Error(), "App not found") || strings.Contains(err.Error(), "Could not find App") {
			tracing.RecordError(span, NoBuilderApp, "no builder app")
			return nil, NoBuilderApp
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

	machines, err := retryFlapsCall(ctx, 5, func() ([]*fly.Machine, error) {
		return flapsClient.List(ctx, "")
	})
	if err != nil {
		if strings.Contains(err.Error(), "App not found") || strings.Contains(err.Error(), "Could not find App") {
			tracing.RecordError(span, NoBuilderApp, "no builder app")
			return nil, NoBuilderApp
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

	_, retErr = retryFlapsCall(ctx, 5, func() (*fly.Machine, error) {
		err := flapsClient.WaitForApp(ctx, app.Name)
		return nil, err
	})
	if retErr != nil {
		tracing.RecordError(span, retErr, "error waiting for builder")
		return nil, nil, fmt.Errorf("waiting for app %s: %w", app.Name, retErr)
	}

	volume, retErr := retryFlapsCall(ctx, 5, func() (*fly.Volume, error) {
		return flapsClient.CreateVolume(ctx, fly.CreateVolumeRequest{
			Name:                "machine_data",
			SizeGb:              fly.IntPointer(50),
			AutoBackupEnabled:   fly.BoolPointer(false),
			ComputeRequirements: &guest,
			Region:              region,
		})
	})
	if retErr != nil {
		return nil, nil, retErr
	}

	defer func() {
		if retErr != nil {
			span.AddEvent("cleaning up new volume due to error")
			flapsClient.DeleteVolume(ctx, volume.ID)
		}
	}()

	mach, retErr = retryFlapsCall(ctx, 5, func() (*fly.Machine, error) {
		return flapsClient.Launch(ctx, fly.LaunchMachineInput{
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
						Autostop:           fly.BoolPointer(false),
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
	})
	if retErr != nil {
		tracing.RecordError(span, retErr, "error launching builder machine")
		return nil, nil, retErr
	}

	return
}

func retryFlapsCall[T *fly.Machine | *fly.Volume | []fly.Volume | []*fly.Machine](ctx context.Context, maxNumRetries int, flapsCallFn func() (T, error)) (returnValue T, err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "retry_flaps_call")
	defer span.End()

	numRetries := 0
	for {
		returnValue, err = flapsCallFn()
		if err == nil {
			return
		}

		var flapsErr *flaps.FlapsError
		if errors.As(err, &flapsErr) {
			span.AddEvent(fmt.Sprintf("server error %d", flapsErr.ResponseStatusCode))
			if flapsErr.ResponseStatusCode >= 500 && flapsErr.ResponseStatusCode < 600 {
				numRetries += 1

				if numRetries >= maxNumRetries {
					tracing.RecordError(span, err, "error retrying flaps call")
					return nil, err
				}
				time.Sleep(1 * time.Second)
				continue
			} else {
				span.AddEvent(fmt.Sprintf("non-server error %d", flapsErr.ResponseStatusCode))
			}
		}

		tracing.RecordError(span, err, "error retrying flaps call")
		return nil, err

	}
}
