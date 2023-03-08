package appconfig

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/terminal"
)

func (cfg *Config) Validate(ctx context.Context) (err error) {
	appNameFromContext := NameFromContext(ctx)

	apiClient := client.FromContext(ctx).API()
	appCompact, err := apiClient.GetAppCompact(ctx, appNameFromContext)

	if err != nil {
		return err
	}

	if appCompact.PlatformVersion == MachinesPlatform {
		return cfg.validateLocally()

	} else {
		parsedCfg, err := apiClient.ParseConfig(ctx, appNameFromContext, cfg.SanitizedDefinition())
		if err != nil {
			return err
		}

		if !parsedCfg.Valid {
			fmt.Println()
			if len(parsedCfg.Errors) > 0 {
				terminal.Errorf("\nConfiguration errors in %s:\n\n", cfg.ConfigFilePath())
			}
			for _, e := range parsedCfg.Errors {
				terminal.Errorf("   %s\n", e)
			}
			fmt.Println()
			return errors.New("error app configuration is not valid")
		}

	}
	return nil
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
