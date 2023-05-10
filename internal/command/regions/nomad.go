package regions

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/slices"

	"github.com/superfly/flyctl/api"
)

func v1RunRegionsAdd(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	input := api.ConfigureRegionsInput{
		AppID:        appName,
		Group:        flag.GetString(ctx, "group"),
		AllowRegions: flag.Args(ctx),
	}

	regions, backupRegions, err := apiClient.ConfigureRegions(ctx, input)
	if err != nil {
		return err
	}

	v1PrintRegions(ctx, regions, backupRegions)

	return nil
}

func runRegionsRemove(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	input := api.ConfigureRegionsInput{
		AppID:       appName,
		Group:       flag.GetString(ctx, "group"),
		DenyRegions: flag.Args(ctx),
	}

	regions, backupRegions, err := apiClient.ConfigureRegions(ctx, input)
	if err != nil {
		return err
	}

	v1PrintRegions(ctx, regions, backupRegions)

	return nil
}

func runRegionsSet(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	// Get the Region List
	regions, _, err := apiClient.ListAppRegions(ctx, appName)
	if err != nil {
		return err
	}

	allowedRegions := flag.Args(ctx)
	var deniedRegions []string

	for _, er := range regions {
		if !slices.Contains(allowedRegions, er.Code) {
			deniedRegions = append(deniedRegions, er.Code)
		}
	}

	input := api.ConfigureRegionsInput{
		AppID:        appName,
		Group:        flag.GetString(ctx, "group"),
		AllowRegions: allowedRegions,
		DenyRegions:  deniedRegions,
	}

	newregions, backupRegions, err := apiClient.ConfigureRegions(ctx, input)
	if err != nil {
		return err
	}

	v1PrintRegions(ctx, newregions, backupRegions)

	return nil
}

func v1RunRegionsList(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	regions, backupRegions, err := apiClient.ListAppRegions(ctx, appName)
	if err != nil {
		return err
	}

	v1PrintRegions(ctx, regions, backupRegions)

	return nil
}

func runRegionsBackup(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	input := api.ConfigureRegionsInput{
		AppID:         appName,
		BackupRegions: flag.Args(ctx),
	}

	regions, backupRegions, err := apiClient.ConfigureRegions(ctx, input)
	if err != nil {
		return err
	}

	v1PrintRegions(ctx, regions, backupRegions)

	return nil
}

func v1PrintRegions(ctx context.Context, regions []api.Region, backupRegions []api.Region) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	if config.FromContext(ctx).JSONOutput {
		data := struct {
			Regions       []api.Region
			BackupRegions []api.Region
		}{
			Regions:       regions,
			BackupRegions: backupRegions,
		}
		render.JSON(io.Out, data)
		return
	}

	verbose := flag.GetBool(ctx, "verbose")

	fmt.Fprintln(io.Out, colorize.Bold("Region Pool: "))
	for _, r := range regions {
		if verbose {
			fmt.Fprintf(io.Out, "%s %s\n", r.Code, r.Name)
		} else {
			fmt.Fprintf(io.Out, "%s\n", r.Code)
		}
	}

	fmt.Fprintln(io.Out, colorize.Bold("Backup Region: "))
	for _, r := range backupRegions {
		if verbose {
			fmt.Fprintf(io.Out, "%s %s\n", r.Code, r.Name)
		} else {
			fmt.Fprintf(io.Out, "%s\n", r.Code)
		}
	}
}
