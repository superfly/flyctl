package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/recipes"
)

func newRecipesCommand(client *client.Client) *Command {
	keystrings := docstrings.Get("recipes")
	cmd := BuildCommandCobra(nil, nil, &cobra.Command{
		Use:   keystrings.Usage,
		Short: keystrings.Short,
		Long:  keystrings.Long,
	}, client)

	newPostgresRecipesCommand(cmd, client)

	return cmd
}

func newPostgresRecipesCommand(parent *Command, client *client.Client) *Command {
	keystrings := docstrings.Get("recipes.postgres")
	pgCmd := BuildCommandCobra(parent, nil, &cobra.Command{
		Use:   keystrings.Usage,
		Short: keystrings.Short,
		Long:  keystrings.Long,
	}, client)

	// Provision
	provisionKeystrings := docstrings.Get("recipes.postgres.provision")
	provisionCmd := BuildCommandKS(pgCmd, runPostgresProvisionRecipe, provisionKeystrings, client, requireSession)
	provisionCmd.AddStringFlag(StringFlagOpts{Name: "name", Description: "the name of the new app"})
	provisionCmd.AddIntFlag(IntFlagOpts{Name: "count", Description: "the total number of in-region Postgres machines", Default: 2})
	provisionCmd.AddStringFlag(StringFlagOpts{Name: "region", Description: "the region to launch the new app in"})
	provisionCmd.AddStringFlag(StringFlagOpts{Name: "volume-size", Description: "the size in GB for volumes"})
	provisionCmd.AddStringFlag(StringFlagOpts{Name: "image-ref", Description: "the target image", Default: "flyio/postgres:14"})
	provisionCmd.AddStringFlag(StringFlagOpts{Name: "password", Description: "the default password for the postgres use"})
	provisionCmd.AddStringFlag(StringFlagOpts{Name: "consul-url", Description: "Opt into using an existing consul as the backend store by specifying the target consul url."})
	provisionCmd.AddStringFlag(StringFlagOpts{Name: "etcd-url", Description: "Opt into using an existing etcd as the backend store by specifying the target etcd url."})

	// Reboot
	rebootKeystrings := docstrings.Get("recipes.postgres.reboot")
	BuildCommandKS(pgCmd, runPostgresRollingRebootRecipe, rebootKeystrings, client, requireSession, requireAppName)

	// Image upgrade
	upgradeKeystrings := docstrings.Get("recipes.postgres.version-upgrade")
	upgradeCmd := BuildCommandKS(pgCmd, runPostgresImageUpgradeRecipe, upgradeKeystrings, client, requireSession, requireAppName)
	upgradeCmd.AddStringFlag(StringFlagOpts{Name: "image-ref", Description: "the target image", Default: "flyio/postgres:14"})

	return pgCmd
}

func runPostgresProvisionRecipe(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	appName := cmdCtx.Config.GetString("name")
	if appName == "" {
		n, err := inputAppName("", false)
		if err != nil {
			return err
		}
		appName = n
	}

	orgSlug := cmdCtx.Config.GetString("organization")
	org, err := selectOrganization(ctx, cmdCtx.Client.API(), orgSlug, nil)
	if err != nil {
		return err
	}

	regionCode := cmdCtx.Config.GetString("region")
	region, err := selectRegion(ctx, cmdCtx.Client.API(), regionCode)
	if err != nil {
		return err
	}

	consulUrl := cmdCtx.Config.GetString("consul-url")
	etcdUrl := cmdCtx.Config.GetString("etcd-url")

	if consulUrl != "" && etcdUrl != "" {
		return fmt.Errorf("consulUrl and etcdUrl may not both be specified.")
	}

	volumeSize := cmdCtx.Config.GetInt("volume-size")
	if volumeSize == 0 {
		s, err := volumeSizeInput(10)
		if err != nil {
			return err
		}
		volumeSize = s
	}

	count := cmdCtx.Config.GetInt("count")
	password := cmdCtx.Config.GetString("password")
	imageRef := cmdCtx.Config.GetString("image-ref")

	p := recipes.NewPostgresProvisionRecipe(cmdCtx, recipes.PostgresProvisionConfig{
		AppName:      appName,
		Count:        count,
		ImageRef:     imageRef,
		Organization: org,
		Password:     password,
		Region:       region.Code,
		VolumeSize:   volumeSize,
		ConsulUrl:    consulUrl,
		EtcdUrl:      etcdUrl,
	})

	return p.Start()
}

func runPostgresImageUpgradeRecipe(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	client := cmdCtx.Client.API()

	app, err := client.GetApp(ctx, cmdCtx.AppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	imageRef := cmdCtx.Config.GetString("image-ref")
	if imageRef == "" {
		return fmt.Errorf("Please specify the target image")
	}

	return recipes.PostgresImageUpgradeRecipe(ctx, app, imageRef)
}

func runPostgresRollingRebootRecipe(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	client := cmdCtx.Client.API()

	app, err := client.GetApp(ctx, cmdCtx.AppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	return recipes.PostgresRollingRebootRecipe(ctx, app)
}
