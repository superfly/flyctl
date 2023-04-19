package status

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/slices"
)

func getFromMetadata(m *api.Machine, key string) string {
	if m.Config != nil && m.Config.Metadata != nil {
		return m.Config.Metadata[key]
	}

	return ""
}

func getProcessgroup(m *api.Machine) string {
	name := m.ProcessGroup()
	if name == "" {
		name = "<default>"
	}

	if len(m.Config.Standbys) > 0 {
		name += "†"
	}
	return name
}

func getReleaseVersion(m *api.Machine) string {
	return getFromMetadata(m, api.MachineConfigMetadataKeyFlyReleaseVersion)
}

// getImage returns the image on the most recent machine released under an app.
func getImage(machines []*api.Machine) (string, error) {
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

func renderMachineStatus(ctx context.Context, app *api.AppCompact, out io.Writer) error {
	var (
		io         = iostreams.FromContext(ctx)
		colorize   = io.ColorScheme()
		client     = client.FromContext(ctx).API()
		jsonOutput = config.FromContext(ctx).JSONOutput
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
		return machines[i].ID > machines[j].ID
	})

	if jsonOutput {
		return renderMachineJSONStatus(ctx, app, machines)
	}

	if app.IsPostgresApp() {
		return renderPGStatus(ctx, app, machines, out)
	}

	// Tracks latest eligible version
	var latest *api.ImageVersion

	var updatable []*api.Machine

	for _, machine := range machines {
		image := fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag)

		latestImage, err := client.GetLatestImageDetails(ctx, image)
		if err != nil {
			if strings.Contains(err.Error(), "Unknown repository") {
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

	managed, unmanaged := []*api.Machine{}, []*api.Machine{}

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

	obj := [][]string{{app.Name, app.Organization.Slug, app.Hostname, image, app.PlatformVersion}}
	if err := render.VerticalTable(out, "App", obj, "Name", "Owner", "Hostname", "Image", "Platform"); err != nil {
		return err
	}

	if len(managed) > 0 {
		hasStandbys := false
		rows := [][]string{}
		for _, machine := range managed {
			if len(machine.Config.Standbys) > 0 {
				hasStandbys = true
			}
			rows = append(rows, []string{
				getProcessgroup(machine),
				machine.ID,
				getReleaseVersion(machine),
				machine.Region,
				machine.State,
				render.MachineHealthChecksSummary(machine),
				machine.UpdatedAt,
			})
		}

		sort.Slice(rows, func(i, j int) bool {
			return slices.Compare(rows[i], rows[j]) < 0
		})

		err := render.Table(out, "Machines", rows, "Process", "ID", "Version", "Region", "State", "Checks", "Last Updated")
		if err != nil {
			return err
		}

		if hasStandbys {
			fmt.Fprintf(out, "  † Standby machine (it will take over only in case of host hardware failure)\n")
		}
	}

	if len(unmanaged) > 0 {
		msg := fmt.Sprintf("Found machines that aren't part of the Fly Apps Platform, run %s to see them.\n", io.ColorScheme().Yellow("fly machines list"))
		fmt.Fprint(out, msg)
	}

	return nil
}

func renderMachineJSONStatus(ctx context.Context, app *api.AppCompact, machines []*api.Machine) error {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API()
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

	machinesToShow := []*api.Machine{}
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

func renderPGStatus(ctx context.Context, app *api.AppCompact, machines []*api.Machine, out io.Writer) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = client.FromContext(ctx).API()
	)

	if len(machines) > 0 {
		if postgres.IsFlex(machines[0]) {
			yes, note := isQuorumMet(machines)
			if !yes {
				fmt.Fprintf(out, colorize.Yellow(note))
			}
		}
	} else {
		fmt.Fprintf(out, "No machines are available on this app %s\n", app.Name)
		return
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

	return render.Table(out, "", rows, "ID", "State", "Role", "Region", "Checks", "Image", "Created", "Updated")
}

func isQuorumMet(machines []*api.Machine) (bool, string) {
	primaryRegion := machines[0].Config.Env["PRIMARY_REGION"]

	totalPrimary := 0
	activePrimary := 0
	total := 0
	inactive := 0

	for _, m := range machines {
		isPrimaryRegion := m.Region == primaryRegion

		if isPrimaryRegion {
			totalPrimary++
		}

		if m.IsActive() {
			if isPrimaryRegion {
				activePrimary++
			}
		} else {
			inactive++
		}

		total++
	}

	quorum := total/2 + 1
	totalActive := (total - inactive)

	// Verify that we meet basic quorum requirements.
	if totalActive <= quorum {
		return false, fmt.Sprintf("WARNING: Cluster size does not meet requirements for HA (expected >= 3, got %d)\n", totalActive)
	}

	// If quorum is met, verify that we have at least 2 active nodes within the primary region.
	if totalActive > 2 && activePrimary < 2 {
		return false, fmt.Sprintf("WARNING: Cluster size within the PRIMARY_REGION %q does not meet requirements for HA (expected >= 2, got %d)\n", primaryRegion, totalPrimary)
	}

	return true, ""
}
