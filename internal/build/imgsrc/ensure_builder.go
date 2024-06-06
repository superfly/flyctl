package imgsrc

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/haikunator"
	"github.com/superfly/flyctl/internal/tracing"
)

func EnsureBuilder(ctx context.Context, org *fly.Organization, region string) (*fly.Machine, *fly.App, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "ensure_builder")
	defer span.End()

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

	if validateBuilderErr != NoBuilderApp {
		span.AddEvent("deleting existing invalid builder")
		client := flyutil.ClientFromContext(ctx)
		err := client.DeleteApp(ctx, builderApp.Name)
		if err != nil {
			tracing.RecordError(span, err, "error deleting invalid builder app")
			return nil, nil, err
		}
	}

	app, machine, err := createBuilder(ctx, org, region)
	return machine, app, err
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
	default:
		return "unknown error validating builder"
	}
}

const (
	NoBuilderApp ValidateBuilderError = iota
	NoBuilderVolume
	InvalidMachineCount
)

func validateBuilder(ctx context.Context, app *fly.App) (*fly.Machine, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "validate_builder")
	defer span.End()

	if app == nil {
		tracing.RecordError(span, NoBuilderApp, "no builder app")
		return nil, NoBuilderApp
	}

	flapsClient := flapsutil.ClientFromContext(ctx)

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
		} else {
			tracing.RecordError(span, err, "error getting volumes")
			return nil, err
		}
	}

	if len(volumes) == 0 {
		tracing.RecordError(span, NoBuilderVolume, "the existing builder app has no volume")
		return nil, NoBuilderVolume
	}

	var machines []*fly.Machine
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

func createBuilder(ctx context.Context, org *fly.Organization, region string) (app *fly.App, mach *fly.Machine, err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "create_builder")
	defer span.End()
	client := flyutil.ClientFromContext(ctx)

	appName := "fly-builder-" + haikunator.Haikunator().Build()
	app, err = client.CreateApp(ctx, fly.CreateAppInput{
		OrganizationID:  org.ID,
		Name:            appName,
		AppRoleID:       "remote-docker-builder",
		Machines:        true,
		PreferredRegion: fly.StringPointer(region),
	})
	if err != nil {
		tracing.RecordError(span, err, "error creating app")
		return nil, nil, err
	}

	defer func() {
		if err != nil {
			span.AddEvent("cleaning up new builder due to error")
			client.DeleteApp(ctx, app.Name)
		}
	}()

	// we want to lauch the machine to the builder
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
		AppCompact: &fly.AppCompact{
			ID:       app.ID,
			Name:     app.Name,
			Status:   app.Status,
			Deployed: app.Deployed,
			Hostname: app.Hostname,
			AppURL:   app.AppURL,
			Organization: &fly.OrganizationBasic{
				ID:       app.Organization.ID,
				Name:     app.Organization.Name,
				Slug:     app.Organization.Slug,
				RawSlug:  app.Organization.RawSlug,
				PaidPlan: app.Organization.PaidPlan,
			},
			PlatformVersion: app.PlatformVersion,
			PostgresAppRole: app.PostgresAppRole,
		},
		OrgSlug: app.Organization.Slug,
	})

	_, err = client.AllocateIPAddress(ctx, appName, "shared_v4", "", org, "")
	if err != nil {
		tracing.RecordError(span, err, "error allocating ip address")
		return nil, nil, err
	}

	guest := fly.MachineGuest{
		CPUKind:  "shared",
		CPUs:     4,
		MemoryMB: 4096,
	}

	err = flapsClient.WaitForApp(ctx, appName)
	if err != nil {
		tracing.RecordError(span, err, "error waiting for app")
		return nil, nil, fmt.Errorf("waiting for app %s: %w", appName, err)
	}

	var volume *fly.Volume
	numRetries := 0
	for {
		volume, err = flapsClient.CreateVolume(ctx, fly.CreateVolumeRequest{
			Name:                "machine_data",
			SizeGb:              fly.IntPointer(50),
			AutoBackupEnabled:   fly.BoolPointer(false),
			ComputeRequirements: &guest,
			Region:              region,
		})
		if err == nil {
			break
		}

		var flapsErr *flaps.FlapsError
		if errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode >= 500 && flapsErr.ResponseStatusCode < 600 {
			span.AddEvent(fmt.Sprintf("non-server error %d", flapsErr.ResponseStatusCode))
			numRetries += 1

			if numRetries >= 3 {
				tracing.RecordError(span, err, "error creating volume")
				return nil, nil, err
			}
		} else {
			tracing.RecordError(span, err, "error creating volume")
			return nil, nil, err
		}
	}

	defer func() {
		if err != nil {
			flapsClient.DeleteVolume(ctx, volume.ID)
		}
	}()

	mach, err = flapsClient.Launch(ctx, fly.LaunchMachineInput{
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
					Name:   appName,
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
	if err != nil {
		tracing.RecordError(span, err, "error launching builder machine")
		return nil, nil, err
	}

	return
}
