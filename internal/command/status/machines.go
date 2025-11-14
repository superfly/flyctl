package status

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func getProcessgroup(m *fly.Machine) string {
	name := m.ProcessGroup()
	if name == "" {
		name = "<default>"
	}

	if len(m.GetConfig().Standbys) > 0 {
		name += "†"
	}

	if m.HostStatus != fly.HostStatusOk {
		name += "💀"
	}
	return name
}

func getReleaseVersion(m *fly.Machine) string {
	return m.GetMetadataByKey(fly.MachineConfigMetadataKeyFlyReleaseVersion)
}

// getImage returns the image on the most recent machine released under an app.
func getImage(machines []*fly.Machine) (string, error) {
	// for context, see this comment https://github.com/superfly/flyctl/pull/1709#discussion_r1110466239
	versionToImage := map[int]string{}
	for _, machine := range machines {
		rv := getReleaseVersion(machine)
		if rv == "" {
			continue
		}

		version, err := strconv.Atoi(rv)
		if err != nil {
			return "", fmt.Errorf("could not parse release version (%s)", rv)
		}

		versionToImage[version] = machine.ImageRefWithVersion()
	}

	highestVersion, latestImage := 0, "-"

	for version, image := range versionToImage {
		if version > highestVersion {
			latestImage = image
			highestVersion = version
		}
	}

	return latestImage, nil
}

func RenderMachineStatus(ctx context.Context, app *fly.AppCompact, out io.Writer) error {
	var (
		io         = iostreams.FromContext(ctx)
		colorize   = io.ColorScheme()
		client     = flyutil.ClientFromContext(ctx)
		jsonOutput = config.FromContext(ctx).JSONOutput
	)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return err
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}

	sort.Slice(machines, func(i, j int) bool {
		return machines[i].ID > machines[j].ID
	})

	if jsonOutput {
		return renderMachineJSONStatus(ctx, app, machines)
	}

	if app.IsPostgresApp() {
		return renderPGStatus(ctx, app, machines, out)
	}

	// Tracks latest eligible version
	var latest *fly.ImageVersion

	var updatable []*fly.Machine

	unknownRepos := map[string]bool{}

	for _, machine := range machines {
		image := fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag)
		// Skip API call for already-seen unknown repos, or default deploy-label prefix.
		if unknownRepos[image] || strings.HasPrefix(machine.ImageRef.Tag, "deployment-") {
			continue
		}

		latestImage, err := client.GetLatestImageDetails(ctx, image, machine.ImageVersion())
		if err != nil {
			if strings.Contains(err.Error(), "Unknown repository") {
				unknownRepos[image] = true
				continue
			}
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

		fmt.Fprintln(out, colorize.Yellow(strings.Join(msgs, "")))
		fmt.Fprintln(out, colorize.Yellow("Run `flyctl image update` to migrate to the latest image version."))
	}

	managed, unmanaged := []*fly.Machine{}, []*fly.Machine{}

	for _, machine := range machines {
		if machine.IsAppsV2() {
			managed = append(managed, machine)
		} else {
			unmanaged = append(unmanaged, machine)
		}
	}

	image, err := getImage(managed)
	if err != nil {
		return err
	}

	obj := [][]string{{app.Name, app.Organization.Slug, app.Hostname, image}}
	if err := render.VerticalTable(out, "App", obj, "Name", "Owner", "Hostname", "Image"); err != nil {
		return err
	}

	if len(managed) > 0 {
		hasStandbys := false
		hasNotOk := false
		rows := [][]string{}
		for _, machine := range managed {
			mConfig := machine.GetConfig()
			if len(mConfig.Standbys) > 0 {
				hasStandbys = true
			}
			if machine.HostStatus != fly.HostStatusOk {
				hasNotOk = true
			}
			var role string

			if v := mConfig.Metadata["role"]; v != "" {
				role = v
			}
			rows = append(rows, []string{
				getProcessgroup(machine),
				machine.ID,
				getReleaseVersion(machine),
				machine.Region,
				machine.State,
				role,
				render.MachineHealthChecksSummary(machine),
				machine.UpdatedAt,
			})
		}

		sort.Slice(rows, func(i, j int) bool {
			return slices.Compare(rows[i], rows[j]) < 0
		})

		err := render.Table(out, "Machines", rows, "Process", "ID", "Version", "Region", "State", "Role", "Checks", "Last Updated")
		if err != nil {
			return err
		}

		if hasStandbys || hasNotOk {
			fmt.Fprint(out, "Notes:\n")
		}
		if hasStandbys {
			fmt.Fprintf(out, "  † Standby machine (it will take over only in case of host hardware failure)\n")
		}
		if hasNotOk {
			fmt.Fprintf(out, "  💀 The machine's host is unreachable\n")
		}
	}

	if len(unmanaged) > 0 {
		msg := fmt.Sprintf("Found machines that aren't part of Fly Launch, run %s to see them.\n", io.ColorScheme().Yellow("fly machines list"))
		fmt.Fprint(out, msg)
	}

	return nil
}

func renderMachineJSONStatus(ctx context.Context, app *fly.AppCompact, machines []*fly.Machine) error {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = flyutil.ClientFromContext(ctx)
	)

	versionQuery := `
		query ($appName: String!) {
			app(name:$appName) {
				currentRelease:currentReleaseUnprocessed {
					version
				}
			}
		}
	`
	req := client.NewRequest(versionQuery)
	req.Var("appName", app.Name)
	resp, err := client.RunWithContext(ctx, req)
	if err != nil {
		return fmt.Errorf("could not get current release for app '%s': %w", app.Name, err)
	}
	version := 0
	if resp.App.CurrentRelease != nil {
		version = resp.App.CurrentRelease.Version
	}

	machinesToShow := []*fly.Machine{}
	if app.IsPostgresApp() {
		machinesToShow = machines
	} else {
		for _, machine := range machines {
			if machine.IsAppsV2() {
				machinesToShow = append(machinesToShow, machine)
			}
		}
	}

	status := map[string]any{
		"ID":              app.ID,
		"Name":            app.Name,
		"Deployed":        app.Deployed,
		"Status":          app.Status,
		"Hostname":        app.Hostname,
		"Version":         version,
		"AppURL":          app.AppURL,
		"Organization":    app.Organization,
		"PlatformVersion": app.PlatformVersion,
		"Machines":        machinesToShow,
	}
	return render.JSON(out, status)
}

func renderPGStatus(ctx context.Context, app *fly.AppCompact, machines []*fly.Machine, out io.Writer) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = flyutil.ClientFromContext(ctx)
	)

	if len(machines) > 0 {
		if postgres.IsFlex(machines[0]) {
			yes, note := isQuorumMet(machines)
			if !yes {
				fmt.Fprint(out, colorize.Yellow(note))
			}
		}
	} else {
		fmt.Fprintf(out, "No machines are available on this app %s\n", app.Name)
		return
	}

	// Tracks latest eligible version
	var latest *fly.ImageVersion
	var updatable []*fly.Machine

	for _, machine := range machines {
		image := fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag)

		latestImage, err := client.GetLatestImageDetails(ctx, image, machine.ImageVersion())

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

		fmt.Fprintln(out, colorize.Yellow(strings.Join(msgs, "")))
		fmt.Fprintln(out, colorize.Yellow("Run `flyctl image update` to migrate to the latest image version."))
	}

	rows := [][]string{}

	for _, machine := range machines {
		role := "unknown"
		for _, check := range machine.Checks {
			if check.Name == "role" {
				if check.Status == fly.Passing {
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

	return render.Table(out, "", rows, "ID", "State", "Role", "Region", "Checks", "Image", "Created", "Updated")
}

func isQuorumMet(machines []*fly.Machine) (bool, string) {
	primaryRegion := machines[0].Config.Env["PRIMARY_REGION"]

	// We are only considering machines in the primary region.
	total := 0
	active := 0

	for _, m := range machines {
		if m.Config.Env["IS_BARMAN"] != "" {
			continue
		}

		isPrimaryRegion := m.Region == primaryRegion

		if isPrimaryRegion {
			total++

			if m.IsActive() {
				active++
			}
		}
	}

	quorum := total/2 + 1

	// Verify that we meet basic quorum requirements.
	if active < quorum {
		return false, fmt.Sprintf("WARNING: Cluster size within your primary region %q does not meet HA requirements. (expected >= 3, got %d)\n", primaryRegion, active)
	}

	return true, ""
}
