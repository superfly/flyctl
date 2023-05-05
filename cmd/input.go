package cmd

import (
	"github.com/AlecAivazis/survey/v2"
)

func confirm(message string) bool {
	confirm := false
	prompt := &survey.Confirm{
		Message: message,
	}
	err := survey.AskOne(prompt, &confirm)
	checkErr(err)

	return confirm
}
