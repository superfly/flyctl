package recipes

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
)

type PostgresProvisionRecipe struct {
	Config PostgresProvisionConfig
	cmdCtx *cmdctx.CmdContext
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
}

func NewPostgresProvisionRecipe(cmdCtx *cmdctx.CmdContext, config PostgresProvisionConfig) *PostgresProvisionRecipe {
	return &PostgresProvisionRecipe{cmdCtx: cmdCtx, Config: config}
}

func (p *PostgresProvisionRecipe) Start() error {
	ctx := p.cmdCtx.Command.Context()
	app, err := p.createApp()
	if err != nil {
		return err
	}

	secrets, err := p.setSecrets(ctx)
	if err != nil {
		return err
	}

	for i := 0; i < p.Config.Count; i++ {
		fmt.Printf("Provisioning %d of %d machines\n", i+1, p.Config.Count)

		machineConf := p.configurePostgres()

		launchInput := api.LaunchMachineInput{
			AppID:  app.ID,
			Region: p.Config.Region,
			Config: &machineConf,
		}

		machine, _, err := p.cmdCtx.Client.API().LaunchMachine(ctx, launchInput)
		if err != nil {
			return err
		}

		if err = WaitForMachineState(ctx, p.cmdCtx.Client, p.Config.AppName, machine.ID, "started"); err != nil {
			return err
		}
	}

	fmt.Printf("Connection string: postgres://postgres:%s@%s.internal:5432\n", secrets["OPERATOR_PASSWORD"], p.Config.AppName)
	return err
}

func (p *PostgresProvisionRecipe) configurePostgres() api.MachineConfig {
	machineConfig := flyctl.NewMachineConfig()

	// Set env
	env := map[string]string{
		"PRIMARY_REGION": p.Config.Region,
	}

	machineConfig.SetEnvVariables(env)
	machineConfig.Config["size"] = "shared-cpu-1x"
	machineConfig.Config["image"] = p.Config.ImageRef
	machineConfig.Config["restart"] = map[string]string{
		"policy": "no",
	}

	// Set mounts
	mounts := make([]map[string]interface{}, 0)
	mounts = append(mounts, map[string]interface{}{
		"volume":    fmt.Sprintf("pg_data_%s", GenerateSecureToken(5)),
		"size_gb":   p.Config.VolumeSize,
		"encrypted": false,
		"path":      "/data",
	})
	machineConfig.Config["mounts"] = mounts

	return api.MachineConfig(machineConfig.Config)
}

func (p *PostgresProvisionRecipe) createApp() (*api.App, error) {

	fmt.Println("Creating app...")
	appInput := api.CreateAppInput{
		OrganizationID:  p.Config.Organization.ID,
		Name:            p.Config.AppName,
		PreferredRegion: &p.Config.Region,
		Runtime:         "FIRECRACKER",
		AppRoleID:       "postgres_cluster",
	}
	return p.cmdCtx.Client.API().CreateApp(p.cmdCtx.Command.Context(), appInput)
}

func (p *PostgresProvisionRecipe) setSecrets(ctx context.Context) (map[string]string, error) {
	fmt.Println("Setting secrets...")

	secrets := map[string]string{
		"FLY_APP_NAME":      p.Config.AppName, // TODO - Move this to web.
		"FLY_REGION":        p.Config.Region,
		"SU_PASSWORD":       GenerateSecureToken(15),
		"REPL_PASSWORD":     GenerateSecureToken(15),
		"OPERATOR_PASSWORD": GenerateSecureToken(15),
	}
	if p.Config.Password != "" {
		secrets["OPERATOR_PASSWORD"] = p.Config.Password
	}
	if p.Config.ConsulUrl != "" {
		secrets["CONSUL_URL"] = p.Config.ConsulUrl
	}
	if p.Config.EtcdUrl != "" {
		secrets["ETCD_URL"] = p.Config.EtcdUrl
	}

	fmt.Printf("%+v\n", secrets)

	_, err := p.cmdCtx.Client.API().SetSecrets(ctx, p.Config.AppName, secrets)

	return secrets, err
}
