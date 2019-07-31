package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
)

func init() {
	authCmd.AddCommand(loginCmd)
}

var username string
var password string

func init() {
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "create a session",
	Run: func(cmd *cobra.Command, args []string) {
		email, err := getEmail()
		if err != nil {
			fmt.Println("Must provide an email")
			os.Exit(1)
		}
		password, err := getPassword()
		if err != nil {
			fmt.Println("Must provide an email")
			os.Exit(1)
		}

		otp, err := getOneTimePassword()
		if err != nil {
			os.Exit(1)
		}

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
			log.Fatalln(err)
			os.Exit(1)
		}

		if resp.StatusCode >= 500 {
			fmt.Println("An unknown server error occured. Please try again.")
			os.Exit(1)
		}

		if resp.StatusCode >= 400 {
			fmt.Println("Incorrect email and password combination")
			os.Exit(1)
		}

		defer resp.Body.Close()

		var result map[string]map[string]map[string]string

		json.NewDecoder(resp.Body).Decode(&result)

		log.Println(result)

		accessToken := result["data"]["attributes"]["access_token"]

		err = flyctl.SetSavedAccessToken(accessToken)
		if err != nil {
			log.Fatalln(err)
		}

		fmt.Println(accessToken)
	},
}

func getEmail() (string, error) {
	prompt := promptui.Prompt{
		Label:    "Email",
		Validate: validatePresence,
	}

	return prompt.Run()
}

func getPassword() (string, error) {
	prompt := promptui.Prompt{
		Label:    "Password",
		Validate: validatePresence,
		Mask:     '*',
	}

	return prompt.Run()
}

func getOneTimePassword() (string, error) {
	prompt := promptui.Prompt{
		Label: "One Time Password (if any)",
		Mask:  '*',
	}

	return prompt.Run()
}

func validatePresence(input string) error {
	if strings.TrimSpace(input) == "" {
		return errors.New("Cannot be empty")
	}
	return nil
}
