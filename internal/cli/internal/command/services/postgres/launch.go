package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newLaunch() (cmd *cobra.Command) {
	const (
		// TODO: document command
		long = `
			Provisions a new Postgres cluster on Machines
		`
		short = "Provisions a new Postgres cluster"
		usage = "launch"
	)

	cmd = command.New(usage, short, long, runLaunch,
		command.RequireSession,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your Postgres app",
		},
		flag.Region(),
		flag.Org(),
		flag.String{
			Name:        "password",
			Shorthand:   "p",
			Description: "The superuser password. The password will be generated for you if you leave this blank",
		},
		flag.String{
			Name:        "vm-size",
			Description: "the size of the VM",
			Default:     "shared-cpu-1x",
		},
		flag.String{
			Name:        "consul-url",
			Description: "Opt into using an existing consul as the backend store by specifying the target consul url.",
		},
		flag.Int{
			Name:        "volume-size",
			Description: "The volume size in GB",
			Default:     10,
		},
		flag.Int{
			Name:        "initial-cluster-size",
			Description: "Initial cluster size",
			Default:     2,
		},
		flag.String{
			Name:        "snapshot-id",
			Description: "Creates the volume with the contents of the snapshot",
		},
	)

	return
}

type Launch struct {
	config LaunchConfig
	client *api.Client
}

type LaunchConfig struct {
	AppName            string
	ConsulURL          string
	ImageRef           string
	InitialClusterSize int
	Organization       *api.Organization
	Password           string
	Region             string
	SnapshotID         string
	VolumeSize         int
	VMSize             string
}

func runLaunch(ctx context.Context) error {
	initialClusterSize := flag.GetInt(ctx, "initial-cluster-size")
	password := flag.GetString(ctx, "password")
	volumeSize := flag.GetInt(ctx, "volume-size")
	vmSize := flag.GetString(ctx, "vm-size")
	snapshotID := flag.GetString(ctx, "snapshot-id")
	consulURL := flag.GetString(ctx, "consul-url")

	name := flag.GetString(ctx, "name")
	if name == "" {
		if err := prompt.String(ctx, &name, "App name", "", true); err != nil {
			return err
		}
	}

	org, err := prompt.Org(ctx, nil)
	if err != nil {
		return err
	}

	region := flag.GetString(ctx, "region")
	if region == "" {
		var r *api.Region
		r, err := prompt.Region(ctx)
		if err != nil {
			return err
		}
		region = r.Code
	}

	client := client.FromContext(ctx).API()

	imageRef, err := client.GetLatestImageTag(ctx, "flyio/postgres")
	if err != nil {
		return err
	}

	config := LaunchConfig{
		AppName:            name,
		ConsulURL:          consulURL,
		InitialClusterSize: initialClusterSize,
		Password:           password,
		Region:             region,
		VolumeSize:         volumeSize,
		VMSize:             vmSize,
		SnapshotID:         snapshotID,
		ImageRef:           imageRef,
		Organization:       org,
	}

	pg := &Launch{
		config: config,
		client: client,
	}

	if err = pg.Launch(ctx); err != nil {
		return fmt.Errorf("failed launching postgres: %w", err)
	}

	return nil
}

func (p *Launch) Launch(ctx context.Context) error {
	app, err := p.createApp(ctx)
	if err != nil {
		return err
	}

	secrets, err := p.setSecrets(ctx)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	for i := 0; i < p.config.InitialClusterSize; i++ {
		fmt.Fprintf(io.Out, "Provisioning %d of %d machines with image %s\n", i+1, p.config.InitialClusterSize, p.config.ImageRef)

		machineConf, err := p.configurePostgres()
		if err != nil {
			return err
		}

		launchInput := api.LaunchMachineInput{
			AppID:   app.ID,
			OrgSlug: p.config.Organization.ID,
			Region:  p.config.Region,
			Config:  machineConf,
		}

		machine, _, err := p.client.LaunchMachine(ctx, launchInput)
		if err != nil {
			return err
		}

		if err = waitForMachineState(ctx, p.client, p.config.AppName, machine.ID, "started"); err != nil {
			return err
		}
	}

	fmt.Fprintf(io.Out, "Provision complete\n\n")

	connStr := fmt.Sprintf("postgres://postgres:%s@%s.internal:5432\n", secrets["OPERATOR_PASSWORD"], p.config.AppName)

	fmt.Fprintf(io.Out, "Any app within the %s organization can connect to this Postgres using the following credentials:\n", p.config.Organization.Name)
	fmt.Fprintf(io.Out, "  Username:    postgres\n")
	fmt.Fprintf(io.Out, "  Password:    %s\n", secrets["OPERATOR_PASSWORD"])
	fmt.Fprintf(io.Out, "  Hostname:    %s.internal\n", p.config.AppName)
	fmt.Fprintf(io.Out, "  Proxy port:  5432\n")
	fmt.Fprintf(io.Out, "  Postgres port:  5433\n")
	fmt.Fprintf(io.Out, "  Connection string:  %s\n", connStr)
	fmt.Fprintf(io.Out, "Save your credentials in a secure place, you won't be able to see them again!\n")

	return nil
}

func (p *Launch) configurePostgres() (*api.MachineConfig, error) {
	machineConfig := api.MachineConfig{}

	// Set env
	machineConfig.Env = map[string]string{
		"PRIMARY_REGION": p.config.Region,
	}

	machineConfig.VMSize = p.config.VMSize
	machineConfig.Image = p.config.ImageRef
	machineConfig.Restart.Policy = api.MachineRestartPolicyNo

	// Set mounts
	volumeHash, err := helpers.RandString(5)
	if err != nil {
		return nil, err
	}

	mount := api.MachineMount{
		Volume:    fmt.Sprintf("pg_data_%s", volumeHash),
		SizeGb:    p.config.VolumeSize,
		Encrypted: false,
		Path:      "/data",
	}
	machineConfig.Mounts = append(machineConfig.Mounts, mount)

	return &machineConfig, nil
}

func (p *Launch) createApp(ctx context.Context) (*api.App, error) {
	fmt.Println("Creating app...")
	appInput := api.CreateAppInput{
		OrganizationID:  p.config.Organization.ID,
		Name:            p.config.AppName,
		PreferredRegion: &p.config.Region,
		Runtime:         "FIRECRACKER",
		AppRoleID:       "postgres_cluster",
	}

	return p.client.CreateApp(ctx, appInput)
}

func (p *Launch) setSecrets(ctx context.Context) (map[string]string, error) {
	fmt.Println("Setting secrets...")

	var suPassword, replPassword, opPassword string
	var err error

	suPassword, err = helpers.RandString(15)
	if err != nil {
		return nil, err
	}

	replPassword, err = helpers.RandString(15)
	if err != nil {
		return nil, err
	}

	opPassword, err = helpers.RandString(15)
	if err != nil {
		return nil, err
	}

	secrets := map[string]string{
		"FLY_APP_NAME":      p.config.AppName, // TODO - Move this to web.
		"FLY_REGION":        p.config.Region,
		"SU_PASSWORD":       suPassword,
		"REPL_PASSWORD":     replPassword,
		"OPERATOR_PASSWORD": opPassword,
	}

	if p.config.ConsulURL == "" {
		consulURL, err := p.generateConsulURL(ctx)
		if err != nil {
			return nil, err
		}
		secrets["FLY_CONSUL_URL"] = consulURL
	} else {
		secrets["CONSUL_URL"] = p.config.ConsulURL
	}

	if p.config.Password != "" {
		secrets["OPERATOR_PASSWORD"] = p.config.Password
	}

	_, err = p.client.SetSecrets(ctx, p.config.AppName, secrets)

	return secrets, err
}

func (p *Launch) generateConsulURL(ctx context.Context) (string, error) {
	data, err := p.client.EnablePostgresConsul(ctx, p.config.AppName)
	if err != nil {
		return "", err
	}

	return data.ConsulURL, nil
}
