// Package deployments implements `fly deployments`, for apps that deploy from GitHub
// through the Fly.io dashboard.
package deployments

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// client is the part of the api.fly.io client these commands use.
type client interface {
	ListDeployments(ctx context.Context, appName string, limit, before int) ([]uiex.Deployment, error)
	GetDeployment(ctx context.Context, appName, id string, wait int) (*uiex.Deployment, error)
	GetDeploymentLogs(ctx context.Context, appName, id, step string, limit int) (*uiex.DeploymentLogs, error)
	GetDeploymentDiagnosis(ctx context.Context, appName, id string, lines int) (string, error)
	CreateDeployment(ctx context.Context, appName string, in uiex.CreateDeploymentRequest) (*uiex.Deployment, error)
	RetryDeployment(ctx context.Context, appName, id string) (*uiex.Deployment, error)
	CancelDeployment(ctx context.Context, appName, id string) (*uiex.Deployment, error)
	LaunchFromRepository(ctx context.Context, appName string, in uiex.LaunchRequest) (*uiex.Deployment, error)
	GetLaunchPlan(ctx context.Context, appName, id string) (*uiex.LaunchPlan, error)
	ConfirmLaunchPlan(ctx context.Context, appName, id string, settings map[string]any) (*uiex.Deployment, error)
}

// clientFromContext is replaced in tests.
var clientFromContext = func(ctx context.Context) (client, error) {
	c, ok := uiexutil.ClientFromContext(ctx).(client)
	if !ok {
		return nil, errors.New("the api.fly.io client is not available")
	}
	return c, nil
}

func New() *cobra.Command {
	const (
		short = "Inspect and start deployments of apps that deploy from GitHub"
		long  = `Inspect and start deployments of apps that deploy from a GitHub repository
through the Fly.io dashboard (deploy on push). These are not the releases
'fly deploy' creates from your machine; see 'fly releases' for those.

To fix a failed deployment, start with 'fly deployments diagnose'.`
	)

	cmd := command.New("deployments", short, long, nil)
	cmd.Aliases = []string{"deployment"}

	cmd.AddCommand(
		newList(),
		newShow(),
		newLogs(),
		newDiagnose(),
		newRedeploy(),
		newRetry(),
		newCancel(),
		newLaunch(),
		newPlan(),
		newConfirm(),
	)

	return cmd
}

func newSubcommand(use, short, long string, fn command.Runner, extra ...flag.Flag) *cobra.Command {
	cmd := command.New(use, short, long, fn, command.RequireSession, command.RequireAppName)
	flag.Add(cmd, append([]flag.Flag{flag.App(), flag.AppConfig()}, extra...)...)
	return cmd
}

var followFlag = flag.Bool{
	Name:        "follow",
	Shorthand:   "f",
	Description: "Wait for the deployment to finish, printing its progress",
}

// idArg is the deployment id argument, "latest" when omitted.
func idArg(ctx context.Context) string {
	if args := flag.Args(ctx); len(args) > 0 {
		return args[0]
	}
	return "latest"
}

func newList() *cobra.Command {
	cmd := newSubcommand("list", "List deployments, newest first", "", runList,
		flag.JSONOutput(),
		flag.Int{Name: "limit", Description: "Number of deployments to list (max 100)", Default: 20},
	)
	cmd.Aliases = []string{"ls"}
	cmd.Args = cobra.NoArgs

	return cmd
}

func runList(ctx context.Context) error {
	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	appName := appconfig.NameFromContext(ctx)

	deployments, err := c.ListDeployments(ctx, appName, flag.GetInt(ctx, "limit"), 0)
	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, deployments)
	}

	rows := make([][]string, 0, len(deployments))
	for _, d := range deployments {
		rows = append(rows, []string{
			fmt.Sprint(d.ID),
			statusText(&d),
			deref(d.Trigger),
			shortCommit(d.Git.Commit),
			format.RelativeTime(d.CreatedAt),
		})
	}

	return render.Table(out, "", rows, "ID", "Status", "Trigger", "Commit", "Created")
}

func newShow() *cobra.Command {
	cmd := newSubcommand("show [ID]", "Show a deployment's steps and errors",
		"Show a deployment (the latest one by default): its steps, errors and the\nrepository settings it built with.",
		runShow, flag.JSONOutput(), followFlag)
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}

func runShow(ctx context.Context) error {
	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	appName := appconfig.NameFromContext(ctx)

	d, err := c.GetDeployment(ctx, appName, idArg(ctx), 0)
	if err != nil {
		return err
	}
	if flag.GetBool(ctx, "follow") {
		if d, err = follow(ctx, c, appName, d); err != nil {
			return err
		}
	}

	return printDeployment(ctx, d)
}

func newLogs() *cobra.Command {
	cmd := newSubcommand("logs [ID]", "Show a deployment's log",
		"Show the log lines the dashboard shows for a deployment (the latest one by default).",
		runLogs, flag.JSONOutput(),
		flag.String{Name: "step", Description: "Only lines from this step, such as build"},
		flag.Int{Name: "limit", Description: "Show the last N lines"},
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runLogs(ctx context.Context) error {
	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	appName := appconfig.NameFromContext(ctx)

	logs, err := c.GetDeploymentLogs(ctx, appName, idArg(ctx), flag.GetString(ctx, "step"), flag.GetInt(ctx, "limit"))
	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, logs)
	}
	for _, line := range logs.Lines {
		fmt.Fprintf(out, "[%s] %s\n", line.Step, line.Message)
	}

	return nil
}

func newDiagnose() *cobra.Command {
	cmd := newSubcommand("diagnose [ID]", "Explain a deployment, for fixing the repository",
		`Print a Markdown brief of a deployment (the latest one by default): the failed
step and where its fix usually is, the errors, the end of the step's log, the
repository settings, the last commit that deployed, and how to deploy again.
Written to be pasted into, or read by, a coding agent.`,
		runDiagnose,
		flag.Int{Name: "lines", Description: "Log lines to include (0-500)", Default: -1},
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDiagnose(ctx context.Context) error {
	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	brief, err := c.GetDeploymentDiagnosis(ctx, appconfig.NameFromContext(ctx), idArg(ctx), flag.GetInt(ctx, "lines"))
	if err != nil {
		return err
	}
	fmt.Fprint(iostreams.FromContext(ctx).Out, brief)

	return nil
}

func newRedeploy() *cobra.Command {
	cmd := newSubcommand("redeploy", "Deploy the app's repository",
		`Deploy the app's GitHub repository, like "Deploy now" in the dashboard. Deploys
the head of the repository settings' branch unless --ref is given.`,
		runRedeploy, flag.JSONOutput(), followFlag,
		flag.String{Name: "ref", Description: "Branch or full commit SHA to deploy"},
		flag.Bool{Name: "replace", Description: "Stop a running deployment and start this one"},
	)
	cmd.Args = cobra.NoArgs

	return cmd
}

func runRedeploy(ctx context.Context) error {
	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	appName := appconfig.NameFromContext(ctx)

	d, err := c.CreateDeployment(ctx, appName, uiex.CreateDeploymentRequest{
		Ref:     flag.GetString(ctx, "ref"),
		Replace: flag.GetBool(ctx, "replace"),
	})
	if err != nil {
		return err
	}

	return started(ctx, c, appName, d)
}

func newRetry() *cobra.Command {
	cmd := newSubcommand("retry [ID]", "Deploy the commit a deployment built again", "",
		runRetry, flag.JSONOutput(), followFlag)
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}

func runRetry(ctx context.Context) error {
	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	appName := appconfig.NameFromContext(ctx)

	d, err := c.RetryDeployment(ctx, appName, idArg(ctx))
	if err != nil {
		return err
	}

	return started(ctx, c, appName, d)
}

func newCancel() *cobra.Command {
	cmd := newSubcommand("cancel [ID]", "Stop a running deployment", "", runCancel, flag.JSONOutput())
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}

func runCancel(ctx context.Context) error {
	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	d, err := c.CancelDeployment(ctx, appconfig.NameFromContext(ctx), idArg(ctx))
	if err != nil {
		return err
	}

	return printDeployment(ctx, d)
}

func newLaunch() *cobra.Command {
	cmd := newSubcommand("launch", "Launch an existing app from a GitHub repository",
		`Link a GitHub repository to an existing app and deploy it for the first time.

If the repository has this app's fly.toml, it is deployed as is. Otherwise Fly.io
detects a plan, commits a Dockerfile and fly.toml to a flyio-* branch, and deploys
that. With --review-plan, check the plan with 'fly deployments plan' and confirm it
with 'fly deployments confirm'.

Pushes to the branch deploy automatically afterwards.`,
		runLaunch, flag.JSONOutput(), followFlag,
		flag.String{Name: "repo", Description: "The GitHub repository, as owner/repo"},
		flag.String{Name: "branch", Description: "Branch to deploy (default: the repository's default branch)"},
		flag.String{Name: "config-path", Description: "Path of fly.toml in the repository"},
		flag.String{Name: "working-directory", Description: "Directory of the app in the repository"},
		flag.Bool{Name: "review-plan", Description: "Wait for 'fly deployments confirm' instead of using the detected plan"},
	)
	cmd.Args = cobra.NoArgs

	return cmd
}

func runLaunch(ctx context.Context) error {
	repo := flag.GetString(ctx, "repo")
	if repo == "" {
		return errors.New("--repo owner/repo is required")
	}

	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	appName := appconfig.NameFromContext(ctx)

	d, err := c.LaunchFromRepository(ctx, appName, uiex.LaunchRequest{
		Repository:       repo,
		Branch:           flag.GetString(ctx, "branch"),
		ConfigPath:       flag.GetString(ctx, "config-path"),
		WorkingDirectory: flag.GetString(ctx, "working-directory"),
		ReviewPlan:       flag.GetBool(ctx, "review-plan"),
	})
	if err != nil {
		var apiErr *uiex.DeploymentsAPIError
		if errors.As(err, &apiErr) && apiErr.ActionRequired != nil {
			return fmt.Errorf("%s\n\nOpen %s, then run this command again", apiErr.Message, apiErr.ActionRequired.URL)
		}
		return err
	}

	if flag.GetBool(ctx, "review-plan") && !config.FromContext(ctx).JSONOutput {
		fmt.Fprintf(iostreams.FromContext(ctx).ErrOut,
			"Deployment %d is detecting a plan. Review it with 'fly deployments plan %d' and confirm it with 'fly deployments confirm %d'.\n",
			d.ID, d.ID, d.ID)
		return printDeployment(ctx, d)
	}

	return started(ctx, c, appName, d)
}

func newPlan() *cobra.Command {
	cmd := newSubcommand("plan [ID]", "Show the plan a launch detected",
		"Show the settings and generated files of a launch deployment's plan.",
		runPlan, flag.JSONOutput())
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}

func runPlan(ctx context.Context) error {
	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	plan, err := c.GetLaunchPlan(ctx, appconfig.NameFromContext(ctx), idArg(ctx))
	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, plan)
	}

	fmt.Fprintf(out, "Deployment %d plan: %s\n", plan.DeploymentID, plan.Status)
	if len(plan.Settings) > 0 {
		fmt.Fprintln(out, "\nSettings:")
		for _, key := range slices.Sorted(maps.Keys(plan.Settings)) {
			fmt.Fprintf(out, "  %s: %v\n", key, plan.Settings[key])
		}
	}
	for _, file := range plan.Files {
		fmt.Fprintf(out, "\n--- %s\n%s", file.Path, file.Content)
		if !strings.HasSuffix(file.Content, "\n") {
			fmt.Fprintln(out)
		}
	}

	return nil
}

func newConfirm() *cobra.Command {
	cmd := newSubcommand("confirm [ID]", "Confirm the plan a launch detected", "",
		runConfirm, flag.JSONOutput(), followFlag,
		flag.String{Name: "region", Description: "Override the primary region"},
		flag.Int{Name: "port", Description: "Override the port the app listens on"},
		flag.String{Name: "vm-size", Description: "Override the VM size, such as shared-cpu-1x"},
		flag.Int{Name: "vm-memory", Description: "Override the VM memory, in MB"},
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runConfirm(ctx context.Context) error {
	c, err := clientFromContext(ctx)
	if err != nil {
		return err
	}
	appName := appconfig.NameFromContext(ctx)

	settings := map[string]any{}
	if v := flag.GetString(ctx, "region"); v != "" {
		settings["region"] = v
	}
	if v := flag.GetInt(ctx, "port"); v > 0 {
		settings["http_service_port"] = v
	}
	if v := flag.GetString(ctx, "vm-size"); v != "" {
		settings["vm_size"] = v
	}
	if v := flag.GetInt(ctx, "vm-memory"); v > 0 {
		settings["vm_memory"] = v
	}

	d, err := c.ConfirmLaunchPlan(ctx, appName, idArg(ctx), settings)
	if err != nil {
		return err
	}

	return started(ctx, c, appName, d)
}

// started prints a deployment that was just started, following it with --follow.
func started(ctx context.Context, c client, appName string, d *uiex.Deployment) error {
	if !config.FromContext(ctx).JSONOutput {
		fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Started deployment %d. %s\n", d.ID, d.DashboardURL)
	}
	if flag.GetBool(ctx, "follow") {
		var err error
		if d, err = follow(ctx, c, appName, d); err != nil {
			return err
		}
	}

	return printDeployment(ctx, d)
}

// follow waits for the deployment to finish, using the API's long-poll, and prints
// each change of status or step.
func follow(ctx context.Context, c client, appName string, d *uiex.Deployment) (*uiex.Deployment, error) {
	errOut := iostreams.FromContext(ctx).ErrOut
	last := ""

	for {
		if now := statusText(d); now != last {
			fmt.Fprintf(errOut, "deployment %d: %s\n", d.ID, now)
			last = now
		}
		if d.Finished() {
			return d, nil
		}

		next, err := c.GetDeployment(ctx, appName, fmt.Sprint(d.ID), 30)
		if err != nil {
			return nil, err
		}
		d = next
	}
}

func printDeployment(ctx context.Context, d *uiex.Deployment) error {
	out := iostreams.FromContext(ctx).Out
	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, d)
	}

	fmt.Fprintf(out, "Deployment %d of %s: %s\n", d.ID, d.App, statusText(d))
	if d.Git.Commit != nil || d.Git.Ref != nil {
		fmt.Fprintf(out, "  Commit:  %s\n", firstNonEmpty(d.Git.Commit, d.Git.Ref))
	}
	if d.Trigger != nil {
		fmt.Fprintf(out, "  Trigger: %s\n", *d.Trigger)
	}
	if d.Release != nil {
		fmt.Fprintf(out, "  Release: v%d\n", d.Release.Version)
	}
	if d.PlanStatus != nil {
		fmt.Fprintf(out, "  Plan:    %s\n", *d.PlanStatus)
	}
	fmt.Fprintf(out, "  URL:     %s\n", d.DashboardURL)

	if len(d.Steps) > 0 {
		fmt.Fprintln(out, "\nSteps:")
		for _, step := range d.Steps {
			duration := ""
			if step.DurationSeconds != nil {
				duration = fmt.Sprintf(" (%.0fs)", *step.DurationSeconds)
			}
			fmt.Fprintf(out, "  %-10s %s%s\n", step.Status, step.Description, duration)
		}
	}
	if len(d.Errors) > 0 {
		fmt.Fprintln(out, "\nErrors:")
		for _, e := range d.Errors {
			fmt.Fprintf(out, "  [%s] %s\n", e.Step, e.Message)
		}
	}
	if d.Failure != nil {
		fmt.Fprintf(out, "\nRun 'fly deployments diagnose %d' for the cause and how to fix it.\n", d.ID)
	}

	return nil
}

func statusText(d *uiex.Deployment) string {
	switch {
	case d.Failure != nil:
		return fmt.Sprintf("failed (%s)", d.Failure.Title)
	case d.Status == "running" && d.CurrentStep != nil:
		return "running: " + *d.CurrentStep
	default:
		return d.Status
	}
}

func shortCommit(commit *string) string {
	if commit == nil {
		return ""
	}
	if len(*commit) > 7 {
		return (*commit)[:7]
	}

	return *commit
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func firstNonEmpty(values ...*string) string {
	for _, v := range values {
		if v != nil && *v != "" {
			return *v
		}
	}

	return ""
}
