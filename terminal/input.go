package terminal

import "github.com/AlecAivazis/survey/v2"

func Confirm(message string) bool {
	confirm := false
	prompt := &survey.Confirm{
		Message: message,
	}
	survey.AskOne(prompt, &confirm)

	return confirm
}
