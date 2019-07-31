package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
)

func newAuthLoginCommand() *cobra.Command {
	login := &authLoginCommand{}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "create a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return login.Run(args)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&login.email, "email", "e", "", "login email")
	f.StringVarP(&login.password, "password", "p", "", "login password")
	f.StringVarP(&login.oneTimePassword, "otp", "o", "", "one time password")

	return cmd
}

type authLoginCommand struct {
	email           string
	password        string
	oneTimePassword string
}

func (cmd *authLoginCommand) Run(args []string) error {
	email := cmd.getEmail()
	if email == "" {
		return fmt.Errorf("Must provide an email")
	}
	password := cmd.getPassword()
	if password == "" {
		return fmt.Errorf("Must provide a password")
	}

	otp := cmd.getOneTimePassword()

	postData, _ := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"attributes": map[string]string{
				"email":    email,
				"password": password,
				"otp":      otp,
			},
		},
	})

	url := fmt.Sprintf("%s/api/v1/sessions", viper.GetString(flyctl.ConfigAPIBaseURL))

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(postData))
	if err != nil {
		return err
	}

	if resp.StatusCode >= 500 {
		return errors.New("An unknown server error occured, please try again")
	}

	if resp.StatusCode >= 400 {
		return errors.New("Incorrect email and password combination")
	}

	defer resp.Body.Close()

	var result map[string]map[string]map[string]string

	json.NewDecoder(resp.Body).Decode(&result)

	accessToken := result["data"]["attributes"]["access_token"]

	if err := flyctl.SetSavedAccessToken(accessToken); err != nil {
		return err
	}

	fmt.Println("Session created")

	return nil
}

func (cmd *authLoginCommand) getEmail() string {
	if cmd.email == "" {
		prompt := promptui.Prompt{
			Label:    "Email",
			Validate: validatePresence,
		}

		if val, err := prompt.Run(); err == nil {
			cmd.email = val
		}
	}

	return cmd.email
}

func (cmd *authLoginCommand) getPassword() string {
	if cmd.password == "" {
		prompt := promptui.Prompt{
			Label:    "Password",
			Validate: validatePresence,
			Mask:     '*',
		}

		if val, err := prompt.Run(); err == nil {
			cmd.password = val
		}
	}

	return cmd.password
}

func (cmd *authLoginCommand) getOneTimePassword() string {
	if cmd.oneTimePassword == "" {
		prompt := promptui.Prompt{
			Label: "One Time Password (if any)",
			Mask:  '*',
		}

		if val, err := prompt.Run(); err == nil {
			cmd.oneTimePassword = val
		}
	}

	return cmd.oneTimePassword
}

func validatePresence(input string) error {
	if strings.TrimSpace(input) == "" {
		return errors.New("Cannot be empty")
	}
	return nil
}
