package launch

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

type launchState struct {
	workingDir string
	configPath string
	plan       *launchPlan
	appConfig  *appconfig.Config
	sourceInfo *scanner.SourceInfo
}

// familyToAppType returns a string that describes the app type based on the source info
// For example, "Dockerfile" apps would return "app" but a rails app would return "Rails app"
func familyToAppType(si *scanner.SourceInfo) string {
	if si == nil {
		return "app"
	}
	switch si.Family {
	case "Dockerfile":
		return "app"
	case "":
		return "app"
	}
	return fmt.Sprintf("%s app", si.Family)
}

func runUi(ctx context.Context) (err error) {

	io := iostreams.FromContext(ctx)

	if err := warnLegacyBehavior(ctx); err != nil {
		return err
	}

	// TODO: Metrics

	state, err := v2BuildPlan(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(
		io.Out,
		"We're about to launch your %s on Fly.io. Here's what you're getting:\n\n%s\n",
		familyToAppType(state.sourceInfo),
		state.plan.Summary(),
	)

	confirm := false
	prompt := &survey.Confirm{
		Message: "Do you want to tweak these settings before proceeding?",
	}
	err = survey.AskOne(prompt, &confirm)
	if err != nil {
		// TODO(allison): This should probably not just return the error
		return err
	}

	if confirm {
		err = state.EditInWebUi(ctx)
		if err != nil {
			return err
		}
	}

	err = state.Launch(ctx)
	if err != nil {
		return err
	}

	return nil
}

// warnLegacyBehavior warns the user if they are using a legacy flag
func warnLegacyBehavior(ctx context.Context) error {
	// TODO(Allison): We probably want to support re-configuring an existing app, but
	// that is different from the launch-into behavior of reuse-app, which basically just deployed.
	if flag.IsSpecified(ctx, "reuse-app") {
		return errors.New("the --reuse-app flag is no longer supported. you are likely looking for 'fly deploy'")
	}
	return nil
}
