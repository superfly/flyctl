package lsvd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newSetup() *cobra.Command {
	const help = "Configure an app for log-structured virtual disks (LSVD)"
	cmd := command.New("setup", help, help, runSetup, command.RequireAppName)
	cmd.Args = cobra.NoArgs
	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runSetup(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	client := flyutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)

	secrets, err := client.GetAppSecrets(ctx, appName)
	if err != nil {
		return err
	}

	var (
		haveKeyID  = false
		haveKey    = false
		haveBucket = false

		newSecrets      = make(map[string]string)
		existingSecrets []string
		deletedSecrets  []string

		keyID       string
		key         string
		serviceType int
		endpoint    string
		region      string
		bucket      string
		deviceSize  int
		mountPoint  string
	)

	for _, secret := range secrets {
		switch secret.Name {
		case "AWS_ACCESS_KEY_ID":
			haveKeyID = true
		case "AWS_SECRET_ACCESS_KEY":
			haveKey = true
		case "FLY_LSVD_S3_BUCKET", "BUCKET_NAME":
			haveBucket = true
		case "FLY_LSVD_S3_ENDPOINT", "AWS_ENDPOINT_URL_S3":
		case "AWS_REGION", "FLY_LSVD_DEVICE_SIZE", "FLY_LSVD_MOUNT_POINT":
		default:
			continue
		}
		existingSecrets = append(existingSecrets, secret.Name)
	}

	if len(existingSecrets) > 0 {
		fmt.Fprintf(io.Out, "Found existing LSVD secrets: %s\n", strings.Join(existingSecrets, ", "))
		overwrite, err := prompt.Confirm(ctx, "Reconfigure overwriting existing secrets?")
		if err != nil {
			return err
		} else if !overwrite {
			return errors.New("LSVD is already configured; not reconfiguring")
		}
		fmt.Fprintln(io.Out)
	}

	fmt.Fprintln(
		io.Out,
		"This will configure S3-backed log-structured virtual disks for your app\n"+
			"by setting several secrets on it.\n\n"+
			"THIS IS AN EXPERIMENTAL FEATURE. It's not ready for production use, and\n"+
			"it's not officially supported. If you run into problems, please get in\n"+
			"touch with us at https://community.fly.io.",
	)
	cont, err := prompt.Confirm(ctx, "Do you wish to continue?")
	if err != nil {
		return err
	} else if !cont {
		return errors.New("not continuing")
	}

	fmt.Fprint(
		io.Out,
		"\nTo begin, you'll need to have a bucket on an S3-compatible object\n"+
			"storage service and credentials that can access it.\n\n",
	)

	reuseCreds := false
	if haveKeyID && haveKey {
		fmt.Fprintln(io.Out, "AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY secrets already exist.")
		reuseCreds, err = prompt.Confirm(ctx, "Reuse these existing credentials?")
		if err != nil {
			return err
		}
	}

	if !reuseCreds {
		serviceTypeOptions := []string{"Tigris", "Amazon S3", "Another S3-compatible service"}
		if err := prompt.Select(ctx, &serviceType, "Which service are you using?", serviceTypeOptions[0], serviceTypeOptions...); err != nil {
			return err
		}

		if serviceTypeOptions[serviceType] != "Tigris" {
			if err := prompt.String(ctx, &keyID, "Enter your access key ID:", "", true); err != nil {
				return err
			}
			if err := prompt.Password(ctx, &key, "Enter your secret access key:", true); err != nil {
				return err
			}
			newSecrets["AWS_ACCESS_KEY_ID"] = keyID
			newSecrets["AWS_SECRET_ACCESS_KEY"] = key
		}

		switch serviceTypeOptions[serviceType] {
		case "Tigris":
			fmt.Fprintln(io.Out, "Creating a new Tigris bucket...")
			// Creating a Tigris bucket sets the following secrets:
			// AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_ENDPOINT_URL_S3, AWS_REGION, BUCKET_NAME
			if err := createTigrisBucket(ctx, appName); err != nil {
				return err
			}
			haveBucket = true
		case "Amazon S3":
			if err := prompt.String(ctx, &region, "Enter your bucket's region:", "", true); err != nil {
				return err
			}
			newSecrets["AWS_REGION"] = region
			deletedSecrets = append(deletedSecrets, "FLY_LSVD_S3_ENDPOINT", "AWS_ENDPOINT_URL_S3")
		case "Another S3-compatible service":
			if err := prompt.String(ctx, &endpoint, "Enter your S3-compatible service's endpoint URL:", "", true); err != nil {
				return err
			}
			newSecrets["AWS_ENDPOINT_URL_S3"] = endpoint
			deletedSecrets = append(deletedSecrets, "AWS_REGION", "FLY_LSVD_S3_ENDPOINT")
		default:
			return &flyerr.GenericErr{
				Err:     "invalid option selected",
				Suggest: "This is a bug. Please report this at https://github.com/superfly/flyctl/issues/new/choose",
			}
		}
	}

	if !haveBucket {
		if err := prompt.String(ctx, &bucket, "Enter your bucket's name:", "", true); err != nil {
			return err
		}
		newSecrets["BUCKET_NAME"] = bucket
	}

	fmt.Fprintln(
		io.Out,
		"\nNext, you'll need to specify the size of your volume. (Be aware that the\n"+
			"LSVD background daemon currently requires 2 MiB of RAM per gigabyte of\n"+
			"volume, so larger volumes will require larger Machines to run.)",
	)
	for {
		if err := prompt.Int(ctx, &deviceSize, "Enter the volume's size (GiB):", 1, true); err != nil {
			return err
		} else if deviceSize > 0 {
			break
		}
		fmt.Fprintln(io.Out, "The volume size must be positive.")
	}
	newSecrets["FLY_LSVD_DEVICE_SIZE"] = strconv.Itoa(deviceSize * 1024 * 1024 * 1024)

	fmt.Fprintln(
		io.Out,
		"\nOptionally, we can automatically create an ext4 filesystem on the volume\n"+
			"and mount it at a specified path. To make use of this, your image must\n"+
			"contain the `mkfs.ext4` binary, which will be executed on the first run.",
	)
	if err := prompt.String(ctx, &mountPoint, "Enter a mount point for the volume (or leave empty to disable):", "", false); err != nil {
		return err
	}
	if mountPoint != "" {
		newSecrets["FLY_LSVD_MOUNT_POINT"] = mountPoint
	} else {
		deletedSecrets = append(deletedSecrets, "FLY_LSVD_MOUNT_POINT")
	}

	if len(deletedSecrets) > 0 {
		_, err = client.UnsetSecrets(ctx, appName, deletedSecrets)
		if err != nil {
			return err
		}
	}

	if len(newSecrets) > 0 {
		_, err = client.SetSecrets(ctx, appName, newSecrets)
		if err != nil {
			return err
		}
	}

	fmt.Fprintln(
		io.Out,
		"\nLSVD is configured! New machines will now be deployed with LSVD enabled.",
	)
	return nil
}

func createTigrisBucket(ctx context.Context, appName string) error {
	_, err := extensions.ProvisionExtension(ctx, extensions.ExtensionParams{
		Provider: "tigris",
		AppName:  appName,
	})
	return err
}
