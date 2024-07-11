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

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	endpointURL.User = url.UserPassword(accessKeyId, accessSecret)
	endpointURL.Path = "/" + bucketName + "/" + bucketDirectory
	config.BarmanSecret = endpointURL.String()

	return nil
}

// backupSourceAppName = config.BarmanRemoteRestoreConfig
// flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
// 	AppName: config.BarmanRemoteRestoreConfig,
// })
// if err != nil {
// 	return err
// }
// ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

// machines, err := flapsClient.ListActive(ctx)
// if err != nil {
// 	return err
// }

// if len(machines) == 0 {
// 	return fmt.Errorf("No active machines")
// }

// enabled := false
// secrets, err := client.GetAppSecrets(ctx, config.BarmanRemoteRestoreConfig)
// if err != nil {
// 	return err
// }

// for _, secret := range secrets {
// 	if secret.Name == BarmanSecretName {
// 		enabled = true
// 		break
// 	}
// }

// if !enabled {
// 	return fmt.Errorf("Backups are not enabled for %s", config.BarmanRemoteRestoreConfig)
// }

// machine := machines[0]

// in := &fly.MachineExecRequest{
// 	Cmd: "bash -c \"echo $AWS_ACCESS_KEY_ID; echo $AWS_SECRET_ACCESS_KEY; echo $BUCKET_NAME; echo $AWS_ENDPOINT_URL_S3\"",
// }

// out, err := flapsClient.Exec(ctx, machine.ID, in)
// if err != nil {
// 	return err
// }
// if out.StdOut == "" {
// 	return fmt.Errorf("AWS_ACCESS_KEY_ID is unset")
// }
// outputLines := strings.Split(strings.TrimSpace(out.StdOut), "\n")
// if len(outputLines) < 4 {
// 	return fmt.Errorf("Invalid output format")
// }
// accessKey := strings.TrimSpace(outputLines[0])
// secretKey := strings.TrimSpace(outputLines[1])
// bucketName := strings.TrimSpace(outputLines[2])
// endpoint := strings.TrimSpace(outputLines[3])

// body := url.QueryEscape("{\"name\":\"restore\",\"buckets_role\":[{\"bucket\":\"" + bucketName + "\",\"role\":\"ReadOnly\"}]}")
// body = "Req=" + body
// req, err := http.NewRequest(http.MethodPost, "https://fly.iam.storage.tigris.dev/?Action=CreateAccessKeyWithBucketsRole", strings.NewReader(body))
// if err != nil {
// 	return err
// }

// req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
// req.Header.Set("accept", "application/json")
// req.SetBasicAuth(accessKey, secretKey)

// region := "auto"
// service := "s3"
// sess := session.Must(session.NewSession(&aws.Config{
// 	Region:      aws.String(region),
// 	Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
// }))
// signer := v4.NewSigner(sess.Config.Credentials)
// _, err = signer.Sign(req, bytes.NewReader([]byte(body)), service, region, time.Now())
// if err != nil {
// 	return err
// }

// res, err := http.DefaultClient.Do(req)
// if err != nil {
// 	return err
// }

// resBody, err := ioutil.ReadAll(res.Body)
// resStr := string(resBody)
// var resMap map[string]interface{}
// err = json.Unmarshal([]byte(resStr), &resMap)
// if err != nil {
// 	return err
// }

// createAccessKeyResult := resMap["CreateAccessKeyResult"].(map[string]interface{})
// newAccessKey := createAccessKeyResult["AccessKey"].(map[string]interface{})
// restoreAccessKey := newAccessKey["AccessKeyId"].(string)
// restoreSecretKey := newAccessKey["SecretAccessKey"].(string)
// bucketDirectory := config.BarmanRemoteRestoreConfig
// endpointUrl, err := url.Parse(endpoint)
// if err != nil {
// 	return err
// }

// values := endpointUrl.Query()
// if config.RestoreTargetName != "" {
// 	values.Set("targetName", config.RestoreTargetName)
// 	endpointUrl.RawQuery = values.Encode()
// } else if config.RestoreTargetTime != "" {
// 	values.Set("targetTime", config.RestoreTargetTime)
// 	if !config.RestoreTargetInclusive {
// 		values.Set("targetInclusive", "false")
// 	}
// 	endpointUrl.RawQuery = values.Encode()
// }

// endpointUrl.User = url.UserPassword(restoreAccessKey, restoreSecretKey)
// endpointUrl.Path = "/" + bucketName + "/" + bucketDirectory
// config.BarmanRemoteRestoreConfig = se
