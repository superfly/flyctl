package status

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func getProcessgroup(m *api.Machine) string {
	var name string

	if m.Config != nil && m.Config.Metadata != nil {
		name = m.Config.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup]
	}

	if name != "" {
		return name
	}

	return "unknown"

}

func renderMachineStatus(ctx context.Context, app *api.AppCompact) error {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = client.FromContext(ctx).API()
	)
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}

	sort.Slice(machines, func(i, j int) bool {
		return machines[i].ID < machines[j].ID
	})

	if app.IsPostgresApp() {
		return renderPGStatus(ctx, app, machines)
	}

	// Tracks latest eligible version
	var latest *api.ImageVersion

	var updatable []*api.Machine

	for _, machine := range machines {
		image := fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag)

		latestImage, err := client.GetLatestImageDetails(ctx, image)

		if err != nil && strings.Contains(err.Error(), "Unknown repository") {
			continue
		}
		if err != nil {
			return fmt.Errorf("unable to fetch latest image details for %s: %w", image, err)
		}

		if latest == nil {
			latest = latestImage
		}

		// Exclude machines that are already running the latest version
		if machine.ImageRef.Digest == latest.Digest {
			continue
		}
		updatable = append(updatable, machine)
	}

	if len(updatable) > 0 {
		msgs := []string{"Updates available:\n\n"}

		for _, machine := range updatable {
			latestStr := fmt.Sprintf("%s:%s (%s)", latest.Repository, latest.Tag, latest.Version)
			msg := fmt.Sprintf("Machine %q %s -> %s\n", machine.ID, machine.ImageRefWithVersion(), latestStr)
			msgs = append(msgs, msg)
		}

		fmt.Fprintln(io.Out, colorize.Yellow(strings.Join(msgs, "")))
		fmt.Fprintln(io.ErrOut, colorize.Yellow("Run `flyctl image update` to migrate to the latest image version."))
	}

	managed, unmanaged := []*api.Machine{}, []*api.Machine{}

	for _, machine := range machines {

		if machine.Config != nil && machine.Config.Metadata != nil {

			if machine.Config.Metadata[api.MachineConfigMetadataKeyFlyPlatformVersion] == api.MachineFlyPlatformVersion2 {
				managed = append(managed, machine)
			} else {
				unmanaged = append(unmanaged, machine)
			}

		}

	}

	obj := [][]string{{app.Name, app.Organization.Slug, app.Hostname, app.PlatformVersion}}
	if err := render.VerticalTable(io.Out, "App", obj, "Name", "Owner", "Hostname", "Platform"); err != nil {
		return err
	}

	if len(managed) > 0 {
		rows := [][]string{}
		for _, machine := range managed {
			rows = append(rows, []string{
				machine.ID,
				machine.State,
				machine.Region,
				getProcessgroup(machine),
				render.MachineHealthChecksSummary(machine),
				machine.ImageRefWithVersion(),
				machine.CreatedAt,
				machine.UpdatedAt,
			})
		}

		err := render.Table(io.Out, "Managed Machines", rows, "ID", "State", "Region", "Process_Group", "Health checks", "Image", "Created", "Updated")
		if err != nil {
			return err
		}
	}

	if len(unmanaged) > 0 {
		rows := [][]string{}
		for _, machine := range unmanaged {
			rows = append(rows, []string{
				machine.ID,
				machine.State,
				machine.Region,
				getProcessgroup(machine),
				render.MachineHealthChecksSummary(machine),
				machine.ImageRefWithVersion(),
				machine.CreatedAt,
				machine.UpdatedAt,
			})
		}

		err := render.Table(io.Out, "Unmanaged Machines", rows, "ID", "State", "Region", "Process_Group", "Health checks", "Image", "Created", "Updated")
		if err != nil {
			return err
		}
	}

	return nil

}

func renderPGStatus(ctx context.Context, app *api.AppCompact, machines []*api.Machine) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = client.FromContext(ctx).API()
	)

	// Tracks latest eligible version
	var latest *api.ImageVersion

	var updatable []*api.Machine

	for _, machine := range machines {
		image := fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag)

		latestImage, err := client.GetLatestImageDetails(ctx, image)

		if err != nil && strings.Contains(err.Error(), "Unknown repository") {
			continue
		}
		if err != nil {
			return fmt.Errorf("unable to fetch latest image details for %s: %w", image, err)
		}

		if latest == nil {
			latest = latestImage
		}

		if latest.Tag != latestImage.Tag {
			return fmt.Errorf("major version mismatch detected")
		}

		// Exclude machines that are already running the latest version
		if machine.ImageRef.Digest == latest.Digest {
			continue
		}
		updatable = append(updatable, machine)
	}

	if len(updatable) > 0 {
		msgs := []string{"Updates available:\n\n"}

		for _, machine := range updatable {
			latestStr := fmt.Sprintf("%s:%s (%s)", latest.Repository, latest.Tag, latest.Version)
			msg := fmt.Sprintf("Machine %q %s -> %s\n", machine.ID, machine.ImageRefWithVersion(), latestStr)
			msgs = append(msgs, msg)
		}

		fmt.Fprintln(io.ErrOut, colorize.Yellow(strings.Join(msgs, "")))
		fmt.Fprintln(io.ErrOut, colorize.Yellow("Run `flyctl image update` to migrate to the latest image version."))
	}

	rows := [][]string{}

	for _, machine := range machines {
		role := "unknown"
		for _, check := range machine.Checks {
			if check.Name == "role" {
				if check.Status == "passing" {
					role = check.Output
				} else {
					role = "error"
				}
			}
		}

		rows = append(rows, []string{
			machine.ID,
			machine.State,
			role,
			machine.Region,
			render.MachineHealthChecksSummary(machine),
			machine.ImageRefWithVersion(),
			machine.CreatedAt,
			machine.UpdatedAt,
		})
	}
	return render.Table(io.Out, "", rows, "ID", "State", "Role", "Region", "Health checks", "Image", "Created", "Updated")
}
