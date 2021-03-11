package docker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
	"golang.org/x/net/context"
)

type DeployOperation struct {
	ctx       context.Context
	apiClient *api.Client
	appName   string
	appConfig *flyctl.AppConfig
}

func NewDeployOperation(ctx context.Context, cmdCtx *cmdctx.CmdContext) (*DeployOperation, error) {
	op := &DeployOperation{
		ctx:       ctx,
		apiClient: cmdCtx.Client.API(),
		appName:   cmdCtx.AppName,
		appConfig: cmdCtx.AppConfig,
	}

	return op, nil
}

func (op *DeployOperation) AppName() string {
	if op.appName != "" {
		return op.appName
	}
	return op.appConfig.AppName
}

type DeploymentStrategy string

const (
	CanaryDeploymentStrategy    DeploymentStrategy = "canary"
	RollingDeploymentStrategy   DeploymentStrategy = "rolling"
	ImmediateDeploymentStrategy DeploymentStrategy = "immediate"
	BlueGreenDeploymentStrategy DeploymentStrategy = "bluegreen"
	DefaultDeploymentStrategy   DeploymentStrategy = ""
)

func ParseDeploymentStrategy(val string) (DeploymentStrategy, error) {
	switch val {
	case "canary":
		return CanaryDeploymentStrategy, nil
	case "rolling":
		return RollingDeploymentStrategy, nil
	case "immediate":
		return ImmediateDeploymentStrategy, nil
	case "bluegreen":
		return BlueGreenDeploymentStrategy, nil
	default:
		return "", fmt.Errorf("Unknown deployment strategy '%s'", val)
	}
}

func (op *DeployOperation) ValidateConfig() (*api.AppConfig, error) {
	if op.appConfig == nil {
		op.appConfig = flyctl.NewAppConfig()
	}

	parsedConfig, err := op.apiClient.ParseConfig(op.appName, op.appConfig.Definition)
	if err != nil {
		return parsedConfig, err
	}

	if !parsedConfig.Valid {
		return parsedConfig, errors.New("App configuration is not valid")
	}

	op.appConfig.Definition = parsedConfig.Definition

	return parsedConfig, nil
}

func (op *DeployOperation) Deploy(imageRef string, strategy DeploymentStrategy) (*api.Release, error) {
	return op.deployImage(imageRef, strategy)
}

func (op *DeployOperation) deployImage(imageTag string, strategy DeploymentStrategy) (*api.Release, error) {
	input := api.DeployImageInput{AppID: op.AppName(), Image: imageTag}
	if strategy != DefaultDeploymentStrategy {
		input.Strategy = api.StringPointer(strings.ToUpper(string(strategy)))
	}

	if op.appConfig != nil && len(op.appConfig.Definition) > 0 {
		x := api.Definition(op.appConfig.Definition)
		input.Definition = &x
	}

	release, err := op.apiClient.DeployImage(input)
	if err != nil {
		return nil, err
	}
	return release, err
}
