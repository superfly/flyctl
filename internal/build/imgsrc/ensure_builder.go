package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/samber/lo"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/haikunator"
)

func EnsureBuilder(ctx context.Context, org *fly.Organization) (*fly.Machine, *fly.App, error) {
	builderApp := org.RemoteBuilderApp
	builderMachine, err := validateBuilder(ctx, builderApp)
	if err == nil {
		return builderMachine, builderApp, nil
	}

	var validateBuilderErr ValidateBuilderError
	if !errors.As(err, &validateBuilderErr) {
		return nil, nil, err
	}

	if validateBuilderErr != NoBuilderApp {
		client := flyutil.ClientFromContext(ctx)
		client.DeleteApp(ctx, builderApp.Name)
	}

	app, machine, err := createBuilder(ctx, org)
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
	if app == nil {
		return nil, NoBuilderApp
	}
	flaps, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: app.Name,
		// TOOD(billy) make a utility function for App -> AppCompact
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
	if err != nil {
		return nil, err
	}

	volumes, err := flaps.GetVolumes(ctx)
	if err != nil {
		return nil, err
	}

	if len(volumes) == 0 {
		return nil, NoBuilderVolume
	}

	machines, err := flaps.List(ctx, "")
	if err != nil {
		return nil, err
	}
	if len(machines) != 1 {
		return nil, InvalidMachineCount
	}

	return machines[0], nil
}

func createBuilder(ctx context.Context, org *fly.Organization) (app *fly.App, mach *fly.Machine, err error) {
	client := flyutil.ClientFromContext(ctx)

	appName := "fly-builder-" + haikunator.Haikunator().Build()
	app, err = client.CreateApp(ctx, fly.CreateAppInput{
		OrganizationID: org.ID,
		Name:           appName,
		AppRoleID:      "remote-docker-builder",
		Machines:       true,
	})
	if err != nil {
		return nil, nil, err
	}

	defer func() {
		if err != nil {
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
		return nil, nil, err
	}

	guest := fly.MachineGuest{
		CPUKind:  "shared",
		CPUs:     4,
		MemoryMB: 4096,
	}

	err = flapsClient.WaitForApp(ctx, appName)
	if err != nil {
		return nil, nil, fmt.Errorf("waiting for app %s: %w", appName, err)
	}

	var volume *fly.Volume
	shoudRetry := true

	err = backoff.Retry(func() error {
		if !shoudRetry {
			return err
		}
		volume, err = flapsClient.CreateVolume(ctx, fly.CreateVolumeRequest{
			Name:                "machine_data",
			SizeGb:              fly.IntPointer(50),
			AutoBackupEnabled:   fly.BoolPointer(false),
			ComputeRequirements: &guest,
		})

		var flapsErr *flaps.FlapsError
		if err != nil && errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode == 503 {
			shoudRetry = false
		}
		return err
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second), 5))
	if err != nil {
		return nil, nil, err
	}

	defer func() {
		if err != nil {
			flapsClient.DeleteVolume(ctx, volume.ID)
		}
	}()

	mach, err = flapsClient.Launch(ctx, fly.LaunchMachineInput{
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
		return nil, nil, err
	}

	return
}
