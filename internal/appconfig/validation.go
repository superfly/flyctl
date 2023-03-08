package appconfig

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/iostreams"
)

func (cfg *Config) Validate(ctx context.Context) (err error, error_info string) {
	io := iostreams.FromContext(ctx)
	appName := NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	if cfg == nil {
		return errors.New("App config file not found"), ""
	}

	platformVersion := NomadPlatform
	app, err := apiClient.GetAppBasic(ctx, appName)
	switch {
	case err == nil:
		platformVersion = app.PlatformVersion
	case strings.Contains(err.Error(), "Could not find App"):
		fmt.Fprintf(io.Out, "WARNING: Failed to fetch platform version: %s\n", err)
	default:
		return err, ""
	}

	fmt.Fprintf(io.Out, "Validating %s (%s)\n", cfg.ConfigFilePath(), platformVersion)

	switch platformVersion {
	case MachinesPlatform:
		err := cfg.EnsureV2Config()
		var extra_info string
		if err == nil {
			extra_info = fmt.Sprintf("%s Configuration is valid\n", aurora.Green("✓"))
			return nil, extra_info
		} else {
			extra_info = fmt.Sprintf("\n   %s%s\n", aurora.Red("✘"), err)
			return errors.New("App configuration is not valid"), extra_info
		}
	case NomadPlatform:
		serverCfg, err := apiClient.ValidateConfig(ctx, appName, cfg.SanitizedDefinition())
		if err != nil {
			return err, ""
		}

		if serverCfg.Valid {
			extra_info := fmt.Sprintf("%s Configuration is valid\n", aurora.Green("✓"))
			return nil, extra_info
		} else {
			extra_info := "\n"
			for _, errStr := range serverCfg.Errors {
				extra_info += fmt.Sprintf("   %s%s\n", aurora.Red("✘"), errStr)
			}
			extra_info += "\n"
			return errors.New("App configuration is not valid"), extra_info
		}
	default:
		return fmt.Errorf("Unknown platform version '%s' for app '%s'", platformVersion, appName), ""
	}

}

func (c *Config) validateLocally() (err error) {
	Validator := validator.New()
	Validator.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		// skip if tag key says it should be ignored
		if name == "-" {
			return ""
		}
		return name
	})

	err = Validator.Struct(c)

	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			if err.Tag() == "required" {
				fmt.Printf("%s is required\n", err.Field())
			} else {
				fmt.Printf("Validation error on %s: %s\n", err.Field(), err.Tag())
			}
		}
	}
	return
}
