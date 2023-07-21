package launch

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

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

	plan, srcInfo, err := v2BuildPlan(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "We're about to launch your %s on Fly.io. Here's what you're getting:\n\n", familyToAppType(srcInfo))
	fmt.Fprintln(io.Out, plan.Summary())

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
		plan, err = v2TweakPlan(ctx, plan)
		if err != nil {
			return err
		}
	}

	err = v2Launch(ctx, plan, srcInfo)
	if err != nil {
		return err
	}

	return nil
}

func warnLegacyBehavior(ctx context.Context) error {
	// TODO(Allison): We probably want to support re-configuring an existing app, but
	// that is different from the launch-into behavior of reuse-app, which basically just deployed.
	if flag.IsSpecified(ctx, "reuse-app") {
		return errors.New("the --reuse-app flag is no longer supported. you are likely looking for 'fly deploy'")
	}
	return nil
}
