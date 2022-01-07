package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
)

func newLaunch() (cmd *cobra.Command) {
	const (
		// TODO: document command
		long = `
`
		// TODO: document command
		short = ""
		usage = "launch [-o ORG] [-r REGION] [NAME]"
	)

	cmd = command.New(usage, short, long, runLaunch,
		command.RequireSession)

	cmd.Args = cobra.RangeArgs(0, 1)

	flag.Add(cmd,
		flag.String{Name: "name", Shorthand: "n", Description: "The name of your Postgres app"},
		flag.Region(),
		flag.String{Name: "password", Shorthand: "p", Description: "The superuser password. The password will be generated for you if you leave this blank"},
		flag.Int{Name: "volume-size", Description: "The volume size in GB", Default: 10},
		flag.String{Name: "vm-size", Description: "the size of the VM"},
		flag.String{Name: "snapshot-id", Description: "Creates the volume with the contents of the snapshot"},
	)

	return
}

type PostgresLaunch struct {
	config PostgresProvisionConfig
	client *client.Client
}

type PostgresProvisionConfig struct {
	AppName      string
	ConsulUrl    string
	Count        int
	EtcdUrl      string
	ImageRef     string
	Organization *api.Organization
	Password     string
	Region       string
	VolumeSize   int
	VMSize       string
	SnapshotId   string
}

func runLaunch(ctx context.Context) (err error) {
	name := flag.GetString(ctx, "name")
	region := flag.GetString(ctx, "region")
	password := flag.GetString(ctx, "password")
	volumeSize := flag.GetInt(ctx, "volume-size")
	vmSize := flag.GetString(ctx, "vm-size")
	snapshotId := flag.GetString(ctx, "snapshot-id")

	config := PostgresProvisionConfig{
		AppName:    name,
		Password:   password,
		Region:     region,
		VolumeSize: volumeSize,
		VMSize:     vmSize,
		SnapshotId: snapshotId,
	}

	client := client.FromContext(ctx)

	pg := &PostgresLaunch{
		config: config,
		client: client,
	}

	return pg.Start(ctx)
}

func (p *PostgresLaunch) Start(ctx context.Context) error {
	app, err := p.createApp(ctx)
	if err != nil {
		return err
	}

	secrets, err := p.setSecrets(ctx)
	if err != nil {
		return err
	}

	for i := 0; i < p.config.Count; i++ {
		fmt.Printf("Provisioning %d of %d machines\n", i+1, p.config.Count)

		machineConf := p.configurePostgres()

		launchInput := api.LaunchMachineInput{
			AppID:  app.ID,
			Region: p.config.Region,
			Config: &machineConf,
		}

		machine, _, err := p.client.API().LaunchMachine(ctx, launchInput)
		if err != nil {
			return err
		}

		if err = WaitForMachineState(ctx, p.client, p.config.AppName, machine.ID, "started"); err != nil {
			return err
		}
	}

	fmt.Printf("Connection string: postgres://postgres:%s@%s.internal:5432\n", secrets["OPERATOR_PASSWORD"], p.config.AppName)
	return err
}

func (p *PostgresLaunch) configurePostgres() api.MachineConfig {
	var err error

	machineConfig := flyctl.NewMachineConfig()
	// Set env
	env := map[string]string{
		"PRIMARY_REGION": p.config.Region,
	}

	machineConfig.SetEnvVariables(env)
	machineConfig.Config["size"] = "shared-cpu-1x"
	machineConfig.Config["image"] = p.config.ImageRef
	machineConfig.Config["restart"] = map[string]string{
		"policy": "no",
	}

	// Set mounts
	var volumeHash string
	if volumeHash, err = helpers.RandString(5); err != nil {
		return nil
	}

	mounts := make([]map[string]interface{}, 0)
	mounts = append(mounts, map[string]interface{}{
		"volume":    fmt.Sprintf("pg_data_%s", volumeHash),
		"size_gb":   p.config.VolumeSize,
		"encrypted": false,
		"path":      "/data",
	})
	machineConfig.Config["mounts"] = mounts

	return api.MachineConfig(machineConfig.Config)
}

func (p *PostgresLaunch) createApp(ctx context.Context) (*api.App, error) {

	fmt.Println("Creating app...")
	appInput := api.CreateAppInput{
		OrganizationID:  p.config.Organization.ID,
		Name:            p.config.AppName,
		PreferredRegion: &p.config.Region,
		Runtime:         "FIRECRACKER",
		AppRoleID:       "postgres_cluster",
	}
	return p.client.API().CreateApp(ctx, appInput)
}

func (p *PostgresLaunch) setSecrets(ctx context.Context) (map[string]string, error) {
	fmt.Println("Setting secrets...")

	var suPassword, replPassword, opPassword string
	var err error

	if suPassword, err = helpers.RandString(15); err != nil {
		return nil, err
	}

	if replPassword, err = helpers.RandString(15); err != nil {
		return nil, err
	}

	if opPassword, err = helpers.RandString(15); err != nil {
		return nil, err
	}

	secrets := map[string]string{
		"FLY_APP_NAME":      p.config.AppName, // TODO - Move this to web.
		"FLY_REGION":        p.config.Region,
		"SU_PASSWORD":       suPassword,
		"REPL_PASSWORD":     replPassword,
		"OPERATOR_PASSWORD": opPassword,
	}

	if p.config.Password != "" {
		secrets["OPERATOR_PASSWORD"] = p.config.Password
	}
	if p.config.ConsulUrl != "" {
		secrets["CONSUL_URL"] = p.config.ConsulUrl
	}
	if p.config.EtcdUrl != "" {
		secrets["ETCD_URL"] = p.config.EtcdUrl
	}

	_, err = p.client.API().SetSecrets(ctx, p.config.AppName, secrets)

	return secrets, err
}
