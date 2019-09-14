package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

func newAuthCommand() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "auth",
			Short: "manage authentication",
			Long:  "Authenticate with Fly (and logout if you need to). Start with the \"login\" subcommand.",
		},
	}

	BuildCommand(cmd, runWhoami, "whoami", "show the currently authenticated user", os.Stdout, true)
	login := BuildCommand(cmd, runLogin, "login", "log in a user", os.Stdout, false)
	login.AddStringFlag(StringFlagOpts{
		Name:        "email",
		Description: "login email",
	})
	login.AddStringFlag(StringFlagOpts{
		Name:        "password",
		Description: "login password",
	})
	login.AddStringFlag(StringFlagOpts{
		Name:        "otp",
		Description: "one time password",
	})

	BuildCommand(cmd, runLogout, "logout", "log out the user", os.Stdout, true)

	return cmd
}

func runWhoami(ctx *CmdContext) error {
	user, err := ctx.FlyClient.GetCurrentUser()
	if err != nil {
		return err
	}
	fmt.Printf("Current user: %s\n", user.Email)
	return nil
}

func runLogin(ctx *CmdContext) error {
	email, _ := ctx.Config.GetString("email")
	if email == "" {
		prompt := &survey.Input{
			Message: "Email:",
		}
		if err := survey.AskOne(prompt, &email, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	password, _ := ctx.Config.GetString("password")
	if password == "" {
		prompt := &survey.Password{
			Message: "Password:",
		}
		if err := survey.AskOne(prompt, &password, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	otp, _ := ctx.Config.GetString("otp")
	if otp == "" {
		prompt := &survey.Password{
			Message: "One Time Password (if any):",
		}
		if err := survey.AskOne(prompt, &otp); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	accessToken, err := api.GetAccessToken(email, password, otp)

	if err != nil {
		return err
	}

	viper.Set(flyctl.ConfigAPIAccessToken, accessToken)

	return flyctl.SaveConfig()
}

func runLogout(ctx *CmdContext) error {
	viper.Set(flyctl.ConfigAPIAccessToken, "")

	if err := flyctl.SaveConfig(); err != nil {
		return err
	}

	fmt.Println("Session removed")

	return nil
}
