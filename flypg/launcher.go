package flypg

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/ssh"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/watch"

	"github.com/superfly/fly-go/flaps"
	iostreams "github.com/superfly/flyctl/iostreams"
)

var (
	volumeName       = "pg_data"
	volumePath       = "/data"
	Duration10s, _   = time.ParseDuration("10s")
	Duration15s, _   = time.ParseDuration("15s")
	CheckPathPg      = "/flycheck/pg"
	CheckPathRole    = "/flycheck/role"
	CheckPathVm      = "/flycheck/vm"
	BarmanSecretName = "S3_ARCHIVE_CONFIG"
)

const (
	ReplicationManager = "repmgr"
	StolonManager      = "stolon"
)

type Launcher struct {
	client flyutil.Client
}

type CreateClusterInput struct {
	AppName                   string
	ConsulURL                 string
	ImageRef                  string
	InitialClusterSize        int
	Organization              *fly.Organization
	Password                  string
	Region                    string
	VolumeSize                *int
	VMSize                    *fly.VMSize
	SnapshotID                *string
	Manager                   string
	Autostart                 bool
	ScaleToZero               bool
	ForkFrom                  string
	BackupEnabled             bool
	BarmanSecret              string
	BarmanRemoteRestoreConfig string
	RestoreTargetName         string
	RestoreTargetTime         string
	RestoreTargetInclusive bool
}

func NewLauncher(client flyutil.Client) *Launcher {
	return &Launcher{
		client: client,
	}
}

func CreateTigrisBucket(ctx context.Context, config *CreateClusterInput) error {
	if !config.BackupEnabled {
		return nil
	}

	var (
		io = iostreams.FromContext(ctx)
	)
	fmt.Fprintln(io.Out, "Creating Tigris bucket for backup storage")

	options := map[string]interface{}{
		"Public":     false,
		"Accelerate": false,
	}
	options["website"] = map[string]interface{}{
		"domain_name": "",
	}
	name := config.AppName + "-postgres"
	params := extensions_core.ExtensionParams{
		AppName:      config.AppName,
		Organization: config.Organization,
		Provider:     "tigris",
		OverrideName: &name,
	}
	params.Options = options

	var extension extensions_core.Extension
	provisionExtension := true
	index := 1

	for provisionExtension {
		var err error
		extension, err = extensions_core.ProvisionExtension(ctx, params)
		if err != nil {
			if strings.Contains(err.Error(), "unavailable") || strings.Contains(err.Error(), "Name has already been taken") {
				name := fmt.Sprintf("%s-postgres-%d", config.AppName, index)
				params.OverrideName = &name
				index++
			} else {
				return err
			}
		} else {
			provisionExtension = false
		}
	}

	environment := extension.Data.Environment
	if environment == nil || reflect.ValueOf(environment).IsNil() {
		return nil
	}

	env := extension.Data.Environment.(map[string]interface{})

	accessKeyId, ok := env["AWS_ACCESS_KEY_ID"].(string)
	if !ok || accessKeyId == "" {
		return fmt.Errorf("AWS_ACCESS_KEY_ID is unset")
	}

	accessSecret, ok := env["AWS_SECRET_ACCESS_KEY"].(string)
	if !ok || accessSecret == "" {
		return fmt.Errorf("AWS_SECRET_ACCESS_KEY is unset")
	}

	endpoint, ok := env["AWS_ENDPOINT_URL_S3"].(string)
	if !ok || endpoint == "" {
		return fmt.Errorf("AWS_ENDPOINT_URL_S3 is unset")
	}

	bucketName, ok := env["BUCKET_NAME"].(string)
	if !ok || bucketName == "" {
		return fmt.Errorf("BUCKET_NAME is unset")
	}

	bucketDirectory := config.AppName

	endpointUrl, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	endpointUrl.User = url.UserPassword(accessKeyId, accessSecret)
	endpointUrl.Path = "/" + bucketName + "/" + bucketDirectory
	config.BarmanSecret = endpointUrl.String()

	return nil
}

// LaunchMachinesPostgres launches a postgres cluster using the machines runtime
func (l *Launcher) LaunchMachinesPostgres(ctx context.Context, config *CreateClusterInput, detach bool) error {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = flyutil.ClientFromContext(ctx)
	)

	// Fail quickly and loudly if someone attempts to restore a backup to a HA cluster.
	if config.BarmanRemoteRestoreConfig != "" && config.InitialClusterSize != 1 {
		return fmt.Errorf("Cannot restore a backup to a cluster with more than 1 instance, pass `--initial-cluster-size 1` to restore")
	}

	// Ensure machines can be started when scaling to zero is enabled
	if config.ScaleToZero {
		config.Autostart = true
	}

	app, err := l.createApp(ctx, config)
	if err != nil {
		return err
	}
	// In case the user hasn't specified a name, use the app name generated by the API
	config.AppName = app.Name

	// if we are not doing a PITR, back up this database to a new bucket
	if config.BarmanRemoteRestoreConfig == "" {
		err = CreateTigrisBucket(ctx, config)
		if err != nil {
			return err
		}
	} else {
		flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
			AppName: config.BarmanRemoteRestoreConfig,
		})
		if err != nil {
			return err
		}
		ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

		machines, err := flapsClient.ListActive(ctx)
		if err != nil {
			return err
		}

		if len(machines) == 0 {
			return fmt.Errorf("No active machines")
		}

		enabled := false
		secrets, err := client.GetAppSecrets(ctx, config.BarmanRemoteRestoreConfig)
		if err != nil {
			return err
		}

		for _, secret := range secrets {
			if secret.Name == BarmanSecretName {
				enabled = true
				break
			}
		}

		if !enabled {
			return fmt.Errorf("Backups are not enabled for %s", config.BarmanRemoteRestoreConfig)
		}

		machine := machines[0]

		in := &fly.MachineExecRequest{
			Cmd: "bash -c \"echo $AWS_ACCESS_KEY_ID; echo $AWS_SECRET_ACCESS_KEY; echo $BUCKET_NAME; echo $AWS_ENDPOINT_URL_S3\"",
		}

		out, err := flapsClient.Exec(ctx, machine.ID, in)
		if err != nil {
			return err
		}
		if out.StdOut == "" {
			return fmt.Errorf("AWS_ACCESS_KEY_ID is unset")
		}
		outputLines := strings.Split(strings.TrimSpace(out.StdOut), "\n")
		if len(outputLines) < 4 {
			return fmt.Errorf("Invalid output format")
		}
		accessKey := strings.TrimSpace(outputLines[0])
		secretKey := strings.TrimSpace(outputLines[1])
		bucketName := strings.TrimSpace(outputLines[2])
		endpoint := strings.TrimSpace(outputLines[3])

		body := url.QueryEscape("{\"name\":\"restore\",\"buckets_role\":[{\"bucket\":\"" + bucketName + "\",\"role\":\"ReadOnly\"}]}")
		body = "Req=" + body
		req, err := http.NewRequest(http.MethodPost, "https://fly.iam.storage.tigris.dev/?Action=CreateAccessKeyWithBucketsRole", strings.NewReader(body))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("accept", "application/json")
		req.SetBasicAuth(accessKey, secretKey)

		region := "auto"
		service := "s3"
		sess := session.Must(session.NewSession(&aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
		}))
		signer := v4.NewSigner(sess.Config.Credentials)
		_, err = signer.Sign(req, bytes.NewReader([]byte(body)), service, region, time.Now())
		if err != nil {
			return err
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		resBody, err := ioutil.ReadAll(res.Body)
		resStr := string(resBody)
		var resMap map[string]interface{}
		err = json.Unmarshal([]byte(resStr), &resMap)
		if err != nil {
			return err
		}

		createAccessKeyResult := resMap["CreateAccessKeyResult"].(map[string]interface{})
		newAccessKey := createAccessKeyResult["AccessKey"].(map[string]interface{})
		restoreAccessKey := newAccessKey["AccessKeyId"].(string)
		restoreSecretKey := newAccessKey["SecretAccessKey"].(string)
		bucketDirectory := config.BarmanRemoteRestoreConfig
		endpointUrl, err := url.Parse(endpoint)
		if err != nil {
			return err
		}

		values := endpointUrl.Query()
		if config.RestoreTargetName != "" {
			values.Set("targetName", config.RestoreTargetName)
			endpointUrl.RawQuery = values.Encode()
		} else if config.RestoreTargetTime != "" {
			values.Set("targetTime", config.RestoreTargetTime)
			if !config.RestoreTargetInclusive {
				values.Set("targetInclusive", "false")
			}
			endpointUrl.RawQuery = values.Encode()
		}

		endpointUrl.User = url.UserPassword(restoreAccessKey, restoreSecretKey)
		endpointUrl.Path = "/" + bucketName + "/" + bucketDirectory
		config.BarmanRemoteRestoreConfig = endpointUrl.String()
		fmt.Println(config.BarmanRemoteRestoreConfig)
	}

	var addr *fly.IPAddress

	if config.Manager == ReplicationManager {
		addr, err = l.client.AllocateIPAddress(ctx, config.AppName, "private_v6", config.Region, config.Organization, "")
		if err != nil {
			return err
		}
	}

	secrets, err := l.setSecrets(ctx, config)
	if err != nil {
		return err
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	nodes := make([]*fly.Machine, 0)

	for i := 0; i < config.InitialClusterSize; i++ {
		machineConf := l.getPostgresConfig(config)

		machineConf.Image = config.ImageRef
		if machineConf.Image == "" {
			imageRepo := "flyio/postgres"

			if config.Manager == ReplicationManager {
				imageRepo = "flyio/postgres-flex"
			}

			imageRef, err := client.GetLatestImageTag(ctx, imageRepo, config.SnapshotID)
			if err != nil {
				return err
			}
			machineConf.Image = imageRef
		}

		concurrency := &fly.MachineServiceConcurrency{
			Type:      "connections",
			HardLimit: 1000,
			SoftLimit: 1000,
		}

		if config.Manager == ReplicationManager {
			var bouncerPort int = 5432
			var pgPort int = 5433
			machineConf.Services = []fly.MachineService{
				{
					Protocol:     "tcp",
					InternalPort: 5432,
					Ports: []fly.MachinePort{
						{
							Port: &bouncerPort,
							Handlers: []string{
								"pg_tls",
							},

							ForceHTTPS: false,
						},
					},
					Concurrency: concurrency,
					Autostart:   &config.Autostart,
				},
				{
					Protocol:     "tcp",
					InternalPort: 5433,
					Ports: []fly.MachinePort{
						{
							Port: &pgPort,
							Handlers: []string{
								"pg_tls",
							},
							ForceHTTPS: false,
						},
					},
					Concurrency: concurrency,
					Autostart:   &config.Autostart,
				},
			}
		}

		snapshot := config.SnapshotID
		verb := "Provisioning"

		if snapshot != nil {
			verb = "Restoring"
			if i > 0 {
				snapshot = nil
			}
		}

		fmt.Fprintf(io.Out, "%s %d of %d machines with image %s\n", verb, i+1, config.InitialClusterSize, machineConf.Image)

		var vol *fly.Volume

		volInput := fly.CreateVolumeRequest{
			Name:                volumeName,
			Encrypted:           fly.Pointer(true),
			RequireUniqueZone:   fly.Pointer(true),
			SnapshotID:          snapshot,
			ComputeRequirements: machineConf.Guest,
			ComputeImage:        machineConf.Image,
		}
		var action string

		if config.ForkFrom != "" {
			// Setting FLY_RESTORED_FROM will treat the provision as a restore.
			machineConf.Env["FLY_RESTORED_FROM"] = config.ForkFrom

			action = "fork"
			volInput.SourceVolumeID = &config.ForkFrom
			volInput.MachinesOnly = fly.Pointer(true)
			volInput.Name = "pg_data"
		} else {
			action = "create"
			volInput.Region = config.Region
			volInput.SizeGb = config.VolumeSize
		}

		vol, err = flapsClient.CreateVolume(ctx, volInput)
		if err != nil {
			return fmt.Errorf("failed to %s volume: %w", action, err)
		}

		machineConf.Mounts = append(machineConf.Mounts, fly.MachineMount{
			Volume: vol.ID,
			Path:   volumePath,
		})

		launchInput := fly.LaunchMachineInput{
			Region: config.Region,
			Config: machineConf,
		}

		machine, err := flapsClient.Launch(ctx, launchInput)
		if err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "Waiting for machine to start...\n")

		waitTimeout := time.Minute * 5
		if snapshot != nil {
			waitTimeout = time.Hour
		}

		err = mach.WaitForStartOrStop(ctx, machine, "start", waitTimeout)
		if err != nil {
			return err
		}
		nodes = append(nodes, machine)

		fmt.Fprintf(io.Out, "Machine %s is %s\n", machine.ID, machine.State)
	}

	if !detach {
		fmt.Fprintln(io.Out, colorize.Green("==> "+"Monitoring health checks"))

		if err := watch.MachinesChecks(ctx, nodes); err != nil {
			return err
		}
		fmt.Fprintln(io.Out)
	}

	connStr := fmt.Sprintf("postgres://postgres:%s@%s.internal:5432\n", secrets["OPERATOR_PASSWORD"], config.AppName)

	if config.Manager == ReplicationManager && addr != nil {
		connStr = fmt.Sprintf("postgres://postgres:%s@%s.flycast:5432\n", secrets["OPERATOR_PASSWORD"], config.AppName)
	}

	fmt.Fprintf(io.Out, "Postgres cluster %s created\n", config.AppName)
	fmt.Fprintf(io.Out, "  Username:    postgres\n")
	fmt.Fprintf(io.Out, "  Password:    %s\n", secrets["OPERATOR_PASSWORD"])
	fmt.Fprintf(io.Out, "  Hostname:    %s.internal\n", config.AppName)
	if addr != nil {
		fmt.Fprintf(io.Out, "  Flycast:     %s\n", addr.Address)
	}
	fmt.Fprintf(io.Out, "  Proxy port:  5432\n")
	fmt.Fprintf(io.Out, "  Postgres port:  5433\n")
	fmt.Fprintf(io.Out, "  Connection string: %s\n", connStr)
	fmt.Fprintln(io.Out, colorize.Italic("Save your credentials in a secure place -- you won't be able to see them again!"))

	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, colorize.Bold("Connect to postgres"))
	fmt.Fprintf(io.Out, "Any app within the %s organization can connect to this Postgres using the above connection string\n", config.Organization.Name)

	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, "Now that you've set up Postgres, here's what you need to understand: https://fly.io/docs/postgres/getting-started/what-you-should-know/")

	// TODO: wait for the cluster to be ready

	return nil
}

func (l *Launcher) getPostgresConfig(config *CreateClusterInput) *fly.MachineConfig {
	machineConfig := fly.MachineConfig{}

	// Set env
	machineConfig.Env = map[string]string{
		"PRIMARY_REGION": config.Region,
	}

	if config.ScaleToZero {
		// TODO make this configurable
		machineConfig.Env["FLY_SCALE_TO_ZERO"] = "1h"
	}

	// Set VM resources
	machineConfig.Guest = &fly.MachineGuest{
		CPUKind:  config.VMSize.CPUClass,
		CPUs:     int(config.VMSize.CPUCores),
		MemoryMB: config.VMSize.MemoryMB,
	}

	// Metrics
	machineConfig.Metrics = &fly.MachineMetrics{
		Path: "/metrics",
		Port: 9187,
	}

	machineConfig.Checks = map[string]fly.MachineCheck{
		"pg": {
			Port:     fly.Pointer(5500),
			Type:     fly.Pointer("http"),
			HTTPPath: &CheckPathPg,
			Interval: &fly.Duration{Duration: Duration15s},
			Timeout:  &fly.Duration{Duration: Duration10s},
		},
		"role": {
			Port:     fly.Pointer(5500),
			Type:     fly.Pointer("http"),
			HTTPPath: &CheckPathRole,
			Interval: &fly.Duration{Duration: Duration15s},
			Timeout:  &fly.Duration{Duration: Duration10s},
		},
		"vm": {
			Port:     fly.Pointer(5500),
			Type:     fly.Pointer("http"),
			HTTPPath: &CheckPathVm,
			Interval: &fly.Duration{Duration: Duration15s},
			Timeout:  &fly.Duration{Duration: Duration10s},
		},
	}

	// Metadata
	machineConfig.Metadata = map[string]string{
		fly.MachineConfigMetadataKeyFlyctlVersion:      buildinfo.Version().String(),
		fly.MachineConfigMetadataKeyFlyPlatformVersion: fly.MachineFlyPlatformVersion2,
		fly.MachineConfigMetadataKeyFlyManagedPostgres: "true",
		"managed-by-fly-deploy":                        "true",
	}

	// Restart policy
	machineConfig.Restart = &fly.MachineRestart{
		Policy: fly.MachineRestartPolicyAlways,
	}

	if config.ScaleToZero {
		machineConfig.Restart.Policy = fly.MachineRestartPolicyOnFailure
		machineConfig.Restart.MaxRetries = 50
	}

	return &machineConfig
}

func (l *Launcher) createApp(ctx context.Context, config *CreateClusterInput) (*fly.AppCompact, error) {
	fmt.Println("Creating app...")
	appInput := fly.CreateAppInput{
		OrganizationID:  config.Organization.ID,
		Name:            config.AppName,
		PreferredRegion: &config.Region,
		AppRoleID:       "postgres_cluster",
	}

	app, err := l.client.CreateApp(ctx, appInput)
	if err != nil {
		return nil, err
	}

	f, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{AppName: app.Name})
	if err != nil {
		return nil, err
	} else if err := f.WaitForApp(ctx, app.Name); err != nil {
		return nil, err
	}

	return &fly.AppCompact{
		ID:       app.ID,
		Name:     app.Name,
		Status:   app.Status,
		Deployed: app.Deployed,
		Hostname: app.Hostname,
		AppURL:   app.AppURL,
		Organization: &fly.OrganizationBasic{
			ID:   app.Organization.ID,
			Slug: app.Organization.Slug,
		},
	}, nil
}

func (l *Launcher) setSecrets(ctx context.Context, config *CreateClusterInput) (map[string]string, error) {
	out := iostreams.FromContext(ctx).Out

	fmt.Fprintf(out, "Setting secrets on app %s...\n", config.AppName)

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

	if config.BarmanSecret != "" {
		secrets[BarmanSecretName] = config.BarmanSecret
	} else if config.BarmanRemoteRestoreConfig != "" {
		secrets["S3_ARCHIVE_REMOTE_RESTORE_CONFIG"] = config.BarmanRemoteRestoreConfig
	}

	if config.Manager == ReplicationManager {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			return nil, err
		}

		// 100 years in hours
		validHours := 876600

		app := fly.App{Name: config.AppName}
		cert, err := l.client.IssueSSHCertificate(ctx, config.Organization, []string{"root", "fly", "postgres"}, []string{app.Name}, &validHours, pub)
		if err != nil {
			return nil, err
		}

		pemkey := ssh.MarshalED25519PrivateKey(priv, "postgres inter-machine ssh")

		secrets["SSH_KEY"] = string(pemkey)
		secrets["SSH_CERT"] = cert.Certificate
	}

	if config.SnapshotID != nil {
		secrets["FLY_RESTORED_FROM"] = *config.SnapshotID
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
