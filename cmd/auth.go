package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
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
			Long:  "manage authentication",
		},
	}

	whoami := BuildCommand(runWhoami, "whoami", "show the currently authenticated user", os.Stdout, true)
	login := BuildCommand(runLogin, "login", "log in a user", os.Stdout, false)
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

	logout := BuildCommand(runLogout, "logout", "log out the user", os.Stdout, true)

	cmd.AddCommand(
		whoami,
		login,
		logout,
	)

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
		prompt := promptui.Prompt{
			Label:    "Email",
			Validate: validatePresence,
		}
		email, _ = prompt.Run()
	}
	if email == "" {
		return fmt.Errorf("Must provide an email")
	}

	password, _ := ctx.Config.GetString("password")
	if password == "" {
		prompt := promptui.Prompt{
			Label:    "Password",
			Mask:     '*',
			Validate: validatePresence,
		}
		password, _ = prompt.Run()
	}
	if password == "" {
		return fmt.Errorf("Must provide a password")
	}

	otp, _ := ctx.Config.GetString("otp")
	if otp == "" {
		prompt := promptui.Prompt{
			Label: "One Time Password (if any)",
			Mask:  '*',
		}
		otp, _ = prompt.Run()
	}

	accessToken, err := api.GetAccessToken(email, password, otp)

	if err != nil {
		return err
	}

	viper.Set(flyctl.ConfigAPIAccessToken, accessToken)

	return saveConfig()
}

func runLogout(ctx *CmdContext) error {
	viper.Set(flyctl.ConfigAPIAccessToken, "")

	if err := saveConfig(); err != nil {
		return err
	}

	fmt.Println("Session removed")

	return nil
}

func validatePresence(input string) error {
	if strings.TrimSpace(input) == "" {
		return errors.New("Cannot be empty")
	}
	return nil
}
