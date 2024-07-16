package flypg

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/superfly/flyctl/iostreams"

	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
)

func CreateTigrisBucket(ctx context.Context, config *CreateClusterInput) error {
	if !config.BackupsEnabled {
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

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	endpointURL.User = url.UserPassword(accessKeyId, accessSecret)
	endpointURL.Path = "/" + bucketName + "/" + bucketDirectory
	config.BarmanSecret = endpointURL.String()

	return nil
}
