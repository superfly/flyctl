package image

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newShow() *cobra.Command {
	const (
		short = "Show image details."
		long  = short + "\n"

		usage = "show"
	)

	cmd := command.New(usage, short, long, runShow,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)

	return cmd
}

func runShow(ctx context.Context) (err error) {
	var (
		client  = client.FromContext(ctx).API()
		io      = iostreams.FromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	var status *api.AppStatus

	if status, err = client.GetAppStatus(ctx, appName, true); err != nil {
		err = fmt.Errorf("failed retrieving app %s: %w", appName, err)

		return
	}

	if !status.Deployed && app.PlatformVersion == "" {
		_, err = fmt.Fprintln(io.Out, "App has not been deployed yet.")

		return
	}

	switch app.PlatformVersion {
	case "nomad":
		return showNomadImage(ctx, app)
	case "machines":
		return showMachineImage(ctx, app)
	}

	return nil
}

func showNomadImage(ctx context.Context, app *api.AppCompact) error {
	var (
		client   = client.FromContext(ctx).API()
		cfg      = config.FromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		// appName  = appv2.NameFromContext(ctx)
	)

	info, err := client.GetImageInfo(ctx, app.Name)
	if err != nil {
		return fmt.Errorf("failed to get image info: %w", err)
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, info.ImageDetails)
	}

	if info.ImageVersionTrackingEnabled && info.ImageUpgradeAvailable {
		current := fmt.Sprintf("%s:%s", info.ImageDetails.Repository, info.ImageDetails.Tag)
		latest := fmt.Sprintf("%s:%s", info.LatestImageDetails.Repository, info.LatestImageDetails.Tag)

		if info.ImageDetails.Version != "" {
			current = fmt.Sprintf("%s %s", current, info.ImageDetails.Version)
		}

		if info.LatestImageDetails.Version != "" {
			latest = fmt.Sprintf("%s %s", latest, info.LatestImageDetails.Version)
		}

		message := fmt.Sprintf("Update available! (%s -> %s)\n", current, latest)
		message += "Run `flyctl image update` to migrate to the latest image version.\n"

		fmt.Fprintln(io.ErrOut, colorize.Yellow(message))
	}

	image := info.ImageDetails

	if image.Version == "" {
		image.Version = "N/A"
	}

	obj := [][]string{
		{
			image.Registry,
			image.Repository,
			image.Tag,
			image.Version,
			image.Digest,
		},
	}

	return render.VerticalTable(io.Out, "Image Details", obj,
		"Registry",
		"Repository",
		"Tag",
		"Version",
		"Digest",
	)
}

func showMachineImage(ctx context.Context, app *api.AppCompact) error {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = client.FromContext(ctx).API()
		cfg      = config.FromContext(ctx)
	)

	flaps, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}

	// if we have machine_id as an arg, we want to show the image for that machine only
	if len(flag.Args(ctx)) > 0 {

		machine, err := flaps.Get(ctx, flag.FirstArg(ctx))
		if err != nil {
			return fmt.Errorf("failed to get machine: %w", err)
		}

		version := "N/A"

		if machine.ImageVersion() != "" {
			version = machine.ImageVersion()
		}

		var labelsString string

		if cfg.JSONOutput {
			json, err := json.Marshal(machine.ImageRef.Labels)
			if err != nil {
				return err
			}
			if string(json) != "null" {
				labelsString = string(json)
			}
		} else {
			for key, val := range machine.ImageRef.Labels {
				labelsString += fmt.Sprintf("%s=%s", key, val)
			}
		}

		obj := map[string]string{
			"MachineID":  machine.ID,
			"Registry":   machine.ImageRef.Registry,
			"Repository": machine.ImageRef.Repository,
			"Tag":        machine.ImageRef.Tag,
			"Version":    version,
			"Digest":     machine.ImageRef.Digest,
			"Labels":     labelsString,
		}

		rows := [][]string{
			{
				machine.ImageRef.Registry,
				machine.ImageRef.Repository,
				machine.ImageRef.Tag,
				version,
				machine.ImageRef.Digest,
				labelsString,
			},
		}

		if cfg.JSONOutput {
			return render.JSON(io.Out, obj)
		}

		return render.VerticalTable(io.Out, "Image Details", rows,
			"Registry",
			"Repository",
			"Tag",
			"Version",
			"Digest",
			"Labels",
		)

	}
	// get machines
	machines, err := flaps.List(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to get machines: %w", err)
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

		if app.IsPostgresApp() {
			// Abort if we detect a postgres machine running a different major version.
			if latest.Tag != latestImage.Tag {
				return fmt.Errorf("major version mismatch detected")
			}
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

	var rows [][]string
	var objs []map[string]string

	for _, machine := range machines {
		image := machine.ImageRef

		version := "N/A"

		if machine.ImageVersion() != "" {
			version = machine.ImageVersion()
		}

		var labelsString string

		if cfg.JSONOutput {
			json, err := json.Marshal(image.Labels)
			if err != nil {
				return err
			}
			if string(json) != "null" {
				labelsString = string(json)
			}
		} else {
			for key, val := range image.Labels {
				labelsString += fmt.Sprintf("%s=%s", key, val)
			}
		}

		objs = append(objs, map[string]string{
			"MachineID":  machine.ID,
			"Registry":   image.Registry,
			"Repository": image.Repository,
			"Tag":        image.Tag,
			"Version":    version,
			"Digest":     image.Digest,
			"Labels":     labelsString,
		})

		rows = append(rows, []string{
			machine.ID,
			image.Registry,
			image.Repository,
			image.Tag,
			version,
			image.Digest,
			labelsString,
		})
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, objs)
	}

	return render.Table(
		io.Out,
		"Image Details",
		rows,
		"Machine ID",
		"Registry",
		"Repository",
		"Tag",
		"Version",
		"Digest",
		"Labels",
	)
}
