package ticket

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

// CreateTicketRequest is the body of the proposed
// POST /api/v1/organizations/:org_slug/support/tickets endpoint. The
// organization is the path parameter rather than a body field.
type CreateTicketRequest struct {
	Subject      string `json:"subject,omitempty"`
	Message      string `json:"message"`
	Priority     string `json:"priority"` // low|high|emergency
	Category     string `json:"category,omitempty"`
	Product      string `json:"product,omitempty"`
	AppName      string `json:"app_name,omitempty"`
	MPGClusterID string `json:"mpg_cluster_id,omitempty"`
}

const (
	messageMinLen = 10
	messageMaxLen = 5000

	defaultChoice = "Something else"
)

var (
	priorities = []string{"low", "high", "emergency"}

	categories = []string{
		"Deployment", "Account Settings", "DNS", "Machines API", "Networking",
		"Performance", "Security", "Sprites", "Proxy", defaultChoice,
	}

	products = []string{
		"Fly Launch", "Fly Apps", "Fly Machines", "Fly GPUs", "Fly Volumes",
		"Sprites", "Managed Postgres", "Fly Postgres (Legacy)",
		"Tigris Object Storage", "Extensions", defaultChoice,
	}
)

func completeStatic(choices []string) func(ctx context.Context, cmd *cobra.Command, args []string, partial string) ([]string, error) {
	return func(ctx context.Context, cmd *cobra.Command, args []string, partial string) ([]string, error) {
		var out []string
		for _, choice := range choices {
			if strings.HasPrefix(strings.ToLower(choice), strings.ToLower(partial)) {
				out = append(out, choice)
			}
		}
		return out, nil
	}
}

func newCreate() *cobra.Command {
	const (
		short = "Open a new support ticket"
		long  = `Open a new support ticket with the Fly.io support team. The ticket body
comes from --message, from stdin when piped, or from an interactive prompt.
Requires an organization with a support plan.`
	)
	cmd := command.New("create", short, long, runCreate,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)
	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.JSONOutput(),
		flag.String{
			Name:        "subject",
			Description: "Short summary used as the ticket title",
		},
		flag.String{
			Name:        "message",
			Shorthand:   "m",
			Description: "Ticket body; reads stdin if piped, prompts otherwise",
		},
		flag.String{
			Name:        "priority",
			Default:     "low",
			Description: "Ticket priority: low, high, or emergency",
		},
		flag.String{
			Name:         "category",
			Default:      defaultChoice,
			Description:  "Issue category, e.g. Deployment, Networking, Performance",
			CompletionFn: completeStatic(categories),
		},
		flag.String{
			Name:         "product",
			Default:      defaultChoice,
			Description:  "Product the ticket concerns, e.g. Fly Machines, Managed Postgres",
			CompletionFn: completeStatic(products),
		},
		flag.String{
			Name:        "mpg-cluster-id",
			Description: "Managed Postgres cluster the ticket concerns",
		},
	)

	return cmd
}

func runCreate(ctx context.Context) error {
	message, err := resolveMessage(ctx)
	if err != nil {
		return err
	}

	priority := flag.GetString(ctx, "priority")
	if err := validateTicket(message, priority); err != nil {
		return err
	}

	org, err := prompt.Org(ctx)
	if err != nil {
		return err
	}

	req := CreateTicketRequest{
		Subject:      flag.GetString(ctx, "subject"),
		Message:      message,
		Priority:     priority,
		Category:     flag.GetString(ctx, "category"),
		Product:      flag.GetString(ctx, "product"),
		AppName:      appconfig.NameFromContext(ctx),
		MPGClusterID: flag.GetString(ctx, "mpg-cluster-id"),
	}

	ios := iostreams.FromContext(ctx)

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(ios.Out, struct {
			DryRun bool                `json:"dry_run"`
			Org    string              `json:"org"`
			Ticket CreateTicketRequest `json:"ticket"`
		}{
			DryRun: true,
			Org:    org.Slug,
			Ticket: req,
		})
	}

	cs := ios.ColorScheme()

	fmt.Fprintf(ios.Out, "%s %s\n", cs.Bold("Organization:"), org.Slug)
	if req.AppName != "" {
		fmt.Fprintf(ios.Out, "%s %s\n", cs.Bold("App:"), req.AppName)
	}
	if req.Subject != "" {
		fmt.Fprintf(ios.Out, "%s %s\n", cs.Bold("Subject:"), req.Subject)
	}
	fmt.Fprintf(ios.Out, "%s %s\n", cs.Bold("Priority:"), req.Priority)
	fmt.Fprintf(ios.Out, "%s %s\n", cs.Bold("Category:"), req.Category)
	fmt.Fprintf(ios.Out, "%s %s\n", cs.Bold("Product:"), req.Product)
	if req.MPGClusterID != "" {
		fmt.Fprintf(ios.Out, "%s %s\n", cs.Bold("MPG cluster:"), req.MPGClusterID)
	}
	fmt.Fprintf(ios.Out, "%s\n%s\n", cs.Bold("Message:"), messagePreview(req.Message))

	fmt.Fprintf(ios.Out, "\n%s Ticket submission isn't wired up yet; this is a preview of what will be sent.\n", cs.WarningIcon())

	return nil
}

// resolveMessage picks the ticket body from --message, piped stdin, or an
// interactive prompt, in that order.
func resolveMessage(ctx context.Context) (string, error) {
	if flag.IsSpecified(ctx, "message") {
		return flag.GetString(ctx, "message"), nil
	}

	ios := iostreams.FromContext(ctx)

	if !ios.IsStdinTTY() {
		b, err := io.ReadAll(ios.In)
		if err != nil {
			return "", fmt.Errorf("failed reading message from stdin: %w", err)
		}
		if msg := strings.TrimRight(string(b), "\n"); msg != "" {
			return msg, nil
		}
	}

	if ios.IsInteractive() {
		var msg string
		if err := prompt.String(ctx, &msg, "Describe the problem:", "", true); err != nil {
			return "", err
		}
		return msg, nil
	}

	return "", prompt.NonInteractiveError("supply a message with --message or pipe it on stdin")
}

func validateTicket(message, priority string) error {
	switch n := len(strings.TrimSpace(message)); {
	case n < messageMinLen:
		return fmt.Errorf("message must be at least %d characters; describe the problem in more detail", messageMinLen)
	case n > messageMaxLen:
		return fmt.Errorf("message must be at most %d characters; trim it down or leave out large logs", messageMaxLen)
	}

	for _, p := range priorities {
		if priority == p {
			return nil
		}
	}
	return fmt.Errorf("invalid priority %q; must be one of: %s", priority, strings.Join(priorities, ", "))
}

func messagePreview(message string) string {
	const maxPreview = 500
	if len(message) > maxPreview {
		return message[:maxPreview] + "…"
	}
	return message
}
