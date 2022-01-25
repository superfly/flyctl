package cmd

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/client"
)

func newChecksCommand(client *client.Client) *Command {
	checksStrings := docstrings.Get("checks")
	cmd := BuildCommandKS(nil, nil, checksStrings, client)

	handlersStrings := docstrings.Get("checks.handlers")
	handlersCmd := BuildCommandKS(cmd, nil, handlersStrings, client)

	handlersListStrings := docstrings.Get("checks.handlers.list")
	listHandlersCmd := BuildCommandKS(handlersCmd, runListChecksHandlers, handlersListStrings, client, requireSession)
	listHandlersCmd.Args = cobra.ExactArgs(1)

	handlersCreateStrings := docstrings.Get("checks.handlers.create")
	createHandlersCmd := BuildCommandKS(handlersCmd, runCreateChecksHandler, handlersCreateStrings, client, requireSession)
	createHandlersCmd.AddStringFlag(StringFlagOpts{Name: "type", Description: "The type of handler to create, can be slack or pagerduty"})
	createHandlersCmd.AddStringFlag(StringFlagOpts{Name: "organization", Shorthand: "o", Description: "The organization to add the handler to"})

	handlersDeleteStrings := docstrings.Get("checks.handlers.delete")
	deleteHandlerCmd := BuildCommandKS(handlersCmd, runDeleteChecksHandler, handlersDeleteStrings, client, requireSession)
	deleteHandlerCmd.Args = cobra.ExactArgs(2)

	checksListStrings := docstrings.Get("checks.list")
	listChecksCmd := BuildCommandKS(cmd, runAppCheckList, checksListStrings, client, requireSession, requireAppName)
	listChecksCmd.AddStringFlag(StringFlagOpts{Name: "check-name", Description: "Filter checks by name"})

	return cmd
}

func runListChecksHandlers(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	slug := cmdCtx.Args[0]

	handlers, err := cmdCtx.Client.API().GetHealthCheckHandlers(ctx, slug)
	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(handlers)
		return nil
	}

	fmt.Fprintf(cmdCtx.Out, "Health Check Handlers for %s\n", slug)

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"Name", "Type"})

	for _, handler := range handlers {
		table.Append([]string{handler.Name, handler.Type})
	}

	table.Render()

	return nil
}

type createHandlerFn func(*cmdctx.CmdContext, *api.Organization, string) error

func runCreateChecksHandler(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	handlerFn := map[string]createHandlerFn{
		"slack":     setSlackChecksHandler,
		"pagerduty": setPagerDutyChecksHandler,
	}

	handlerType := cmdCtx.Config.GetString("type")
	fn, ok := handlerFn[handlerType]
	if !ok {
		return fmt.Errorf("\"%s\" is not a valid handler type", handlerType)
	}

	orgSlug := cmdCtx.Config.GetString("organization")

	org, err := selectOrganization(ctx, cmdCtx.Client.API(), orgSlug, nil)
	if err != nil {
		return err
	}

	name := cmdCtx.Config.GetString("name")
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

	return fn(cmdCtx, org, name)
}

func setSlackChecksHandler(cmdCtx *cmdctx.CmdContext, org *api.Organization, name string) error {
	ctx := cmdCtx.Command.Context()

	webhookURL := cmdCtx.Config.GetString("webhook-url")
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

	slackChannel := cmdCtx.Config.GetString("slack-channel")
	if slackChannel == "" {
		prompt := &survey.Input{
			Message: "Slack Channel (defaults to webhook's configured channel):",
		}
		if err := survey.AskOne(prompt, &slackChannel); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	// slackUsername, _ := ctx.Config.GetString("slack-username")
	// if slackUsername == "" {
	// 	prompt := &survey.Input{
	// 		Message: "Slack Username:",
	// 	}
	// 	if err := survey.AskOne(prompt, &slackUsername); err != nil {
	// 		if isInterrupt(err) {
	// 			return nil
	// 		}
	// 	}
	// }

	// slackIconURL, _ := ctx.Config.GetString("slack-icon-url")
	// if slackIconURL == "" {
	// 	prompt := &survey.Input{
	// 		Message: "Slack Icon URL:",
	// 	}
	// 	if err := survey.AskOne(prompt, &slackIconURL); err != nil {
	// 		if isInterrupt(err) {
	// 			return nil
	// 		}
	// 	}
	// }

	input := api.SetSlackHandlerInput{
		OrganizationID:  org.ID,
		Name:            name,
		SlackWebhookURL: webhookURL,
	}
	if slackChannel != "" {
		input.SlackChannel = api.StringPointer(slackChannel)
	}
	// if slackUsername != "" {
	// 	input.SlackUsername = api.StringPointer(slackUsername)
	// }
	// if slackIconURL != "" {
	// 	input.SlackIconURL = api.StringPointer(slackIconURL)
	// }

	handler, err := cmdCtx.Client.API().SetSlackHealthCheckHandler(ctx, input)

	if err != nil {
		return err
	}

	fmt.Fprintf(cmdCtx.Out, "Created %s handler named %s\n", handler.Type, handler.Name)

	return nil
}

func setPagerDutyChecksHandler(cmdCtx *cmdctx.CmdContext, org *api.Organization, name string) error {
	ctx := cmdCtx.Command.Context()

	pagerDutyToken := cmdCtx.Config.GetString("pagerduty-token")
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

	handler, err := cmdCtx.Client.API().SetPagerdutyHealthCheckHandler(ctx, input)

	if err != nil {
		return err
	}

	fmt.Fprintf(cmdCtx.Out, "Created %s handler named %s\n", handler.Type, handler.Name)

	return nil
}

func runDeleteChecksHandler(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	org, err := cmdCtx.Client.API().FindOrganizationBySlug(ctx, cmdCtx.Args[0])
	if err != nil {
		return err
	}
	handlerName := cmdCtx.Args[1]

	err = cmdCtx.Client.API().DeleteHealthCheckHandler(ctx, org.ID, handlerName)

	if err != nil {
		return err
	}

	fmt.Fprintf(cmdCtx.Out, "Handler \"%s\" deleted from organization %s\n", handlerName, org.Slug)

	return nil
}

func runAppCheckList(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	var nameFilter *string

	if val := cmdCtx.Config.GetString("check-name"); val != "" {
		nameFilter = api.StringPointer(val)
	}

	checks, err := cmdCtx.Client.API().GetAppHealthChecks(ctx, cmdCtx.AppName, nameFilter, nil, api.BoolPointer(true))
	if err != nil {
		return err
	}

	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(checks)
		return nil
	}

	fmt.Fprintf(cmdCtx.Out, "Health Checks for %s\n", cmdCtx.AppName)

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"Name", "Status", "Allocation", "Region", "Type", "Last Updated", "Output"})

	for _, check := range checks {
		// format output
		var formattedOutput string
		oldOutput := strings.ReplaceAll(check.Output, "\n", "")
		oldOutput = strings.ReplaceAll(oldOutput, "] ", "]")
		v := strings.Split(oldOutput, "[✓]")
		for _, attr := range v {
			formattedOutput += fmt.Sprintf("%s[✓]\n", attr)
		}

		table.Append([]string{check.Name, check.Status, check.Allocation.IDShort, check.Allocation.Region, check.Type, presenters.FormatRelativeTime(check.UpdatedAt), formattedOutput})
	}

	table.Render()

	return nil
}
