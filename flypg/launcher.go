package flypg

import (
	"context"
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"

	machines "github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/spinner"
	"github.com/superfly/flyctl/internal/watch"

	"github.com/superfly/flyctl/flaps"
	iostreams "github.com/superfly/flyctl/iostreams"
)

var (
	volumeName = "pg_data"
	volumePath = "/data"
)

type Launcher struct {
	client *api.Client
}

type CreateClusterInput struct {
	AppName            string
	ConsulURL          string
	ImageRef           string
	InitialClusterSize int
	Organization       *api.Organization
	Password           string
	Region             string
	VolumeSize         *int
	VMSize             *string
	SnapshotID         string
}

func NewLauncher(client *api.Client) *Launcher {
	return &Launcher{
		client: client,
	}
}

// Launches a postgres cluster using the machines runtime
func (l *Launcher) LaunchMachinesPostgres(ctx context.Context, config *CreateClusterInput) error {
	var (
		client = client.FromContext(ctx).API()
	)

	app, err := l.createApp(ctx, config)
	if err != nil {
		return err
	}

	secrets, err := l.setSecrets(ctx, config)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)

	flaps, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}

	for i := 0; i < config.InitialClusterSize; i++ {
		fmt.Fprintf(io.Out, "Provisioning %d of %d machines with image %s\n", i+1, config.InitialClusterSize, config.ImageRef)

		machineConf := l.getPostgresConfig(config)

		volInput := api.CreateVolumeInput{
			AppID:             app.ID,
			Name:              volumeName,
			Region:            config.Region,
			SizeGb:            *config.VolumeSize,
			Encrypted:         false,
			RequireUniqueZone: false,
		}

		vol, err := l.client.CreateVolume(ctx, volInput)
		if err != nil {
			return err
		}

		machineConf.Mounts = append(machineConf.Mounts, api.MachineMount{
			Volume:    vol.ID,
			Path:      volumePath,
			SizeGb:    *config.VolumeSize,
			Encrypted: false,
		})

		imageRef, err := client.GetLatestImageTag(ctx, "flyio/postgres")
		if err != nil {
			return err
		}

		machineConf.Image = imageRef

		launchInput := api.LaunchMachineInput{
			AppID:   app.ID,
			OrgSlug: config.Organization.ID,
			Region:  config.Region,
			Config:  machineConf,
		}

		machine, err := flaps.Launch(ctx, launchInput)
		if err != nil {
			return err
		}

		err = machines.WaitForStart(ctx, flaps, machine)
		if err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Machine %s is %s\n", machine.ID, machine.State)

	}

	connStr := fmt.Sprintf("postgres://postgres:%s@%s.internal:5432\n", secrets["OPERATOR_PASSWORD"], config.AppName)

	fmt.Fprintf(io.Out, "  Username:    postgres\n")
	fmt.Fprintf(io.Out, "  Password:    %s\n", secrets["OPERATOR_PASSWORD"])
	fmt.Fprintf(io.Out, "  Hostname:    %s.internal\n", config.AppName)
	fmt.Fprintf(io.Out, "  Proxy port:  5432\n")
	fmt.Fprintf(io.Out, "  Postgres port:  5433\n")
	fmt.Fprintln(io.Out, aurora.Italic("Save your credentials in a secure place, you won't be able to see them again!"))

	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, aurora.Bold("Connect to postgres"))
	fmt.Fprintf(io.Out, "Any app within the %s organization can connect to this Postgres using the following credentials:\n", config.Organization.Name)
	fmt.Fprintf(io.Out, "For example: %s\n", connStr)

	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, "Now you've setup postgres, here's what you need to understand: https://fly.io/docs/reference/postgres-whats-next/")

	// TODO: wait for the cluster to be ready

	return nil
}

// Launches a postgres cluster using the nomad runtime
func (l *Launcher) LaunchNomadPostgres(ctx context.Context, config *CreateClusterInput) (err error) {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)
	)

	if config.ImageRef == "" {
		api.StringPointer("flyio/postgres")
	}

	input := api.CreatePostgresClusterInput{
		Name:           config.AppName,
		OrganizationID: config.Organization.ID,
		Region:         &config.Region,
		ImageRef:       &config.ImageRef,
		Count:          &config.InitialClusterSize,
		Password:       &config.Password,
		VMSize:         config.VMSize,
		VolumeSizeGB:   config.VolumeSize,
	}

	if config.SnapshotID != "" {
		input.SnapshotID = &config.SnapshotID
	}

	s := spinner.Run(io, "Launching...")

	payload, err := client.CreatePostgresCluster(ctx, input)
	if err != nil {
		return err
	}
	s.StopWithMessage(fmt.Sprintf("Postgres cluster %s created\n", payload.App.Name))

	fmt.Fprintf(io.Out, "  Username:    %s\n", payload.Username)
	fmt.Fprintf(io.Out, "  Password:    %s\n", payload.Password)
	fmt.Fprintf(io.Out, "  Hostname:    %s.internal\n", payload.App.Name)
	fmt.Fprintf(io.Out, "  Proxy Port:  5432\n")
	fmt.Fprintf(io.Out, "  Postgres Port: 5433\n")
	fmt.Fprintln(io.Out, aurora.Italic("Save your credentials in a secure place, you won't be able to see them again!"))

	if !flag.GetDetach(ctx) {
		if err := watch.Deployment(ctx, payload.App.Name, ""); err != nil {
			return err
		}
	}

	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, aurora.Bold("Connect to postgres"))
	fmt.Fprintf(io.Out, "Any app within the %s organization can connect to postgres using the above credentials and the hostname \"%s.internal.\"\n", config.Organization.Name, payload.App.Name)
	fmt.Fprintf(io.Out, "For example: postgres://%s:%s@%s.internal:%d\n", payload.Username, payload.Password, payload.App.Name, 5432)

	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, "Now you've setup postgres, here's what you need to understand: https://fly.io/docs/reference/postgres-whats-next/")

	return
}

func (l *Launcher) getPostgresConfig(config *CreateClusterInput) *api.MachineConfig {
	machineConfig := api.MachineConfig{}

	// Set env
	machineConfig.Env = map[string]string{
		"PRIMARY_REGION": config.Region,
	}

	machineConfig.VMSize = *config.VMSize
	machineConfig.Restart.Policy = api.MachineRestartPolicyAlways

	return &machineConfig
}

func (l *Launcher) createApp(ctx context.Context, config *CreateClusterInput) (*api.AppCompact, error) {
	fmt.Println("Creating app...")
	appInput := api.CreateAppInput{
		OrganizationID:  config.Organization.ID,
		Name:            config.AppName,
		PreferredRegion: &config.Region,
		AppRoleID:       "postgres_cluster",
	}

	app, err := l.client.CreateApp(ctx, appInput)
	if err != nil {
		return nil, err
	}

	return &api.AppCompact{
		ID:       app.ID,
		Name:     app.Name,
		Status:   app.Status,
		Deployed: app.Deployed,
		Hostname: app.Hostname,
		AppURL:   app.AppURL,
		Organization: &api.OrganizationBasic{
			ID:   app.Organization.ID,
			Slug: app.Organization.Slug,
		},
	}, nil
}

func (l *Launcher) setSecrets(ctx context.Context, config *CreateClusterInput) (map[string]string, error) {
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
		"SU_PASSWORD":       suPassword,
		"REPL_PASSWORD":     replPassword,
		"OPERATOR_PASSWORD": opPassword,
	}

	if config.ConsulURL == "" {
		consulURL, err := l.generateConsulURL(ctx, config)
		if err != nil {
			return nil, err
		}
		secrets["FLY_CONSUL_URL"] = consulURL
	} else {
		secrets["CONSUL_URL"] = config.ConsulURL
	}

	if config.Password != "" {
		secrets["OPERATOR_PASSWORD"] = config.Password
	}

	_, err = l.client.SetSecrets(ctx, config.AppName, secrets)

	return secrets, err
}

func (l *Launcher) generateConsulURL(ctx context.Context, config *CreateClusterInput) (string, error) {
	data, err := l.client.EnablePostgresConsul(ctx, config.AppName)
	if err != nil {
		return "", err
	}

	return data.ConsulURL, nil
}
