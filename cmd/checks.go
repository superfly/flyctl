package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/helpers"
)

func newChecksCommand() *Command {
	checksStrings := docstrings.Get("checks")
	cmd := BuildCommandKS(nil, nil, checksStrings, os.Stdout)

	handlersStrings := docstrings.Get("checks.handlers")
	handlersCmd := BuildCommandKS(cmd, nil, handlersStrings, os.Stdout)

	handlersListStrings := docstrings.Get("checks.handlers.list")
	listHandlersCmd := BuildCommandKS(handlersCmd, runListChecksHandlers, handlersListStrings, os.Stdout, requireSession)
	listHandlersCmd.Args = cobra.ExactArgs(1)

	handlersCreateStrings := docstrings.Get("checks.handlers.create")
	createHandlersCmd := BuildCommandKS(handlersCmd, runCreateChecksHandler, handlersCreateStrings, os.Stdout, requireSession)
	createHandlersCmd.AddStringFlag(StringFlagOpts{Name: "type", Description: "The type of handler to create, can be slack or ???"})

	handlersDeleteStrings := docstrings.Get("checks.handlers.delete")
	deleteHandlerCmd := BuildCommandKS(handlersCmd, runDeleteChecksHandler, handlersDeleteStrings, os.Stdout, requireSession)
	deleteHandlerCmd.Args = cobra.ExactArgs(2)

	return cmd
}

func runListChecksHandlers(ctx *cmdctx.CmdContext) error {
	slug := ctx.Args[0]

	handlers, err := ctx.Client.API().GetHealthCheckHandlers(slug)
	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(handlers)
		return nil
	}

	fmt.Fprintf(ctx.Out, "Health Check Handlers for %s\n", slug)

	table := helpers.MakeSimpleTable(ctx.Out, []string{"Name", "Type"})

	for _, handler := range handlers {
		table.Append([]string{handler.Name, handler.Type})
	}

	table.Render()

	return nil
}

func runCreateChecksHandler(ctx *cmdctx.CmdContext) error {
	handlerType, _ := ctx.Config.GetString("type")
	switch handlerType {
	case "slack":
		return setSlackChecksHandler(ctx)
	case "pagerduty":
		return setPagerDutyChecksHandler(ctx)
	}
	return fmt.Errorf(`"%s" is not a valid handler type`, handlerType)
}

func setSlackChecksHandler(ctx *cmdctx.CmdContext) error {
	org, err := selectOrganization(ctx.Client.API(), "")
	if err != nil {
		return err
	}

	name, _ := ctx.Config.GetString("name")
	if name == "" {
		prompt := &survey.Input{
			Message: "Name:",
		}
		if err := survey.AskOne(prompt, &name, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	webhookURL, _ := ctx.Config.GetString("webhook-url")
	if webhookURL == "" {
		prompt := &survey.Input{
			Message: "Webhook URL:",
		}
		if err := survey.AskOne(prompt, &webhookURL, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	slackChannel, _ := ctx.Config.GetString("slack-channel")
	if slackChannel == "" {
		prompt := &survey.Input{
			Message: "Slack Channel:",
		}
		if err := survey.AskOne(prompt, &slackChannel); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	slackUsername, _ := ctx.Config.GetString("slack-username")
	if slackUsername == "" {
		prompt := &survey.Input{
			Message: "Slack Username:",
			Default: "Fly",
		}
		if err := survey.AskOne(prompt, &slackUsername); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	slackIconURL, _ := ctx.Config.GetString("slack-icon-url")
	if slackIconURL == "" {
		prompt := &survey.Input{
			Message: "Slack Icon URL:",
		}
		if err := survey.AskOne(prompt, &slackIconURL); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	input := api.SetSlackHandlerInput{
		OrganizationID:  org.ID,
		Name:            name,
		SlackWebhookURL: webhookURL,
	}

	if slackChannel != "" {
		input.SlackChannel = api.StringPointer(slackChannel)
	}

	if slackUsername != "" {
		input.SlackUsername = api.StringPointer(slackUsername)
	}

	if slackIconURL != "" {
		input.SlackIconURL = api.StringPointer(slackIconURL)
	}

	handler, err := ctx.Client.API().SetSlackHealthCheckHandler(input)

	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.Out, "Created %s handler named %s\n", handler.Type, handler.Name)

	return nil
}

func setPagerDutyChecksHandler(ctx *cmdctx.CmdContext) error {
	org, err := selectOrganization(ctx.Client.API(), "")
	if err != nil {
		return err
	}

	name, _ := ctx.Config.GetString("name")
	if name == "" {
		prompt := &survey.Input{
			Message: "Name:",
		}
		if err := survey.AskOne(prompt, &name, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	pagerDutyToken, _ := ctx.Config.GetString("pagerduty-token")
	if pagerDutyToken == "" {
		prompt := &survey.Input{
			Message: "PagerDuty Token:",
		}
		if err := survey.AskOne(prompt, &pagerDutyToken, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	input := api.SetPagerdutyHandlerInput{
		OrganizationID: org.ID,
		Name:           name,
		PagerdutyToken: pagerDutyToken,
	}

	handler, err := ctx.Client.API().SetPagerdutyHealthCheckHandler(input)

	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.Out, "Created %s handler named %s\n", handler.Type, handler.Name)

	return nil
}

func runDeleteChecksHandler(ctx *cmdctx.CmdContext) error {
	org, err := ctx.Client.API().FindOrganizationBySlug(ctx.Args[0])
	if err != nil {
		return err
	}
	handlerName := ctx.Args[1]

	err = ctx.Client.API().DeleteHealthCheckHandler(org.ID, handlerName)

	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.Out, `Handler "%s" deleted from organization %s\n`, handlerName, org.Slug)

	return nil
}
