package checks

import (
	"context"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func runListChecksHandlers(ctx context.Context) error {
	out := iostreams.FromContext(ctx).Out
	web := client.FromContext(ctx).API()
	org := flag.FirstArg(ctx)

	handlers, err := web.GetHealthCheckHandlers(ctx, org)
	if err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, handlers)
	}

	fmt.Fprintf(out, "Health Check Handlers for %s\n", org)
	table := helpers.MakeSimpleTable(out, []string{"Name", "Type"})
	for _, handler := range handlers {
		table.Append([]string{handler.Name, handler.Type})
	}
	table.Render()
	return nil
}

type createHandlerFn func(context.Context, *api.Organization, string) error

func runCreateChecksHandler(ctx context.Context) error {
	web := client.FromContext(ctx).API()
	orgSlug := flag.GetString(ctx, "organization")

	handlerFn := map[string]createHandlerFn{
		"slack":     setSlackChecksHandler,
		"pagerduty": setPagerDutyChecksHandler,
	}

	handlerType := flag.GetString(ctx, "type")
	fn, ok := handlerFn[handlerType]
	if !ok {
		return fmt.Errorf("\"%s\" is not a valid handler type", handlerType)
	}

	org, err := selectOrganization(ctx, web, orgSlug)
	if err != nil {
		return err
	}

	name := flag.GetString(ctx, "name")
	if name == "" {
		prompt := &survey.Input{Message: "Name:"}
		if err := survey.AskOne(prompt, &name, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	return fn(ctx, org, name)
}

func setSlackChecksHandler(ctx context.Context, org *api.Organization, name string) error {
	out := iostreams.FromContext(ctx).Out
	web := client.FromContext(ctx).API()

	webhookURL := flag.GetString(ctx, "webhook-url")
	if webhookURL == "" {
		prompt := &survey.Input{Message: "Webhook URL:"}
		if err := survey.AskOne(prompt, &webhookURL, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	slackChannel := flag.GetString(ctx, "slack-channel")
	if slackChannel == "" {
		prompt := &survey.Input{Message: "Slack Channel (defaults to webhook's configured channel):"}
		if err := survey.AskOne(prompt, &slackChannel); err != nil {
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
	handler, err := web.SetSlackHealthCheckHandler(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Created %s handler named %s\n", handler.Type, handler.Name)
	return nil
}

func setPagerDutyChecksHandler(ctx context.Context, org *api.Organization, name string) error {
	out := iostreams.FromContext(ctx).Out
	web := client.FromContext(ctx).API()

	pagerDutyToken := flag.GetString(ctx, "pagerduty-token")
	if pagerDutyToken == "" {
		prompt := &survey.Input{Message: "PagerDuty Token:"}
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
	handler, err := web.SetPagerdutyHealthCheckHandler(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Created %s handler named %s\n", handler.Type, handler.Name)
	return nil
}

func runDeleteChecksHandler(ctx context.Context) error {
	out := iostreams.FromContext(ctx).Out
	web := client.FromContext(ctx).API()
	orgSlug := flag.Args(ctx)[0]
	handlerName := flag.Args(ctx)[1]

	org, err := web.GetOrganizationBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	err = web.DeleteHealthCheckHandler(ctx, org.ID, handlerName)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Handler \"%s\" deleted from organization %s\n", handlerName, org.Slug)
	return nil
}
