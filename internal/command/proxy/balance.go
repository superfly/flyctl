package proxy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flyproxy"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newBalance() *cobra.Command {
	var (
		long  = strings.Trim(`Runs the load balancing logic for an app through our proxy and shows how it might choose an instance to route to.`, "\n")
		short = `Load balancing simulation for an app`
		usage = "balance"
	)

	cmd := command.New(usage, short, long, runBalance,
		command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
	)

	return cmd
}

func runBalance(ctx context.Context) (err error) {
	apiClient := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)
	orgSlug := flag.GetString(ctx, "org")

	if orgSlug != "" {
		_, err := apiClient.GetOrganizationBySlug(ctx, orgSlug)
		if err != nil {
			return err
		}
	}

	if appName == "" && orgSlug == "" {
		org, err := prompt.Org(ctx)
		if err != nil {
			return err
		}
		orgSlug = org.Slug
	}

	if appName != "" {
		app, err := apiClient.GetAppBasic(ctx, appName)
		if err != nil {
			return err
		}
		orgSlug = app.Organization.Slug
	}

	client, err := flyproxy.New(ctx, orgSlug)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	balanced, err := client.Balance(ctx, appName)

	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "%d instances were eligible for balancing %s.\n\n", balanced.Total, appName)

	rows := [][]string{}

	rejections := make(map[string]string)

	if balanced.Chosen != nil {
		rows = append(rows, generateRow(balanced.Chosen, true, rejections, colorize))
	}

	for _, instance := range balanced.Rejected {
		rows = append(rows, generateRow(instance, false, rejections, colorize))
	}

	if len(rows) > 0 {
		_ = render.Table(io.Out, fmt.Sprintf("Balancing response for %s", appName), rows, "✓", "ID", "State", "Region", "Healthy", "Load", "RTT", "Rejection")

		if len(rejections) > 0 {
			fmt.Fprintln(io.Out, "")
			fmt.Fprintf(io.Out, "Rejection reasons descriptions:\n")
			for id, desc := range rejections {
				fmt.Fprintf(io.Out, "  - %s: %s\n", colorize.Bold(id), desc)
			}
		}
	} else {
		fmt.Fprintf(io.Out, "No instances found for balancing.\n")
	}

	return nil
}

func generateRow(instance *flyproxy.BalancedInstance, chosen bool, rejections map[string]string, colorize *iostreams.ColorScheme) []string {
	healthy := "yes"
	if !instance.NodeHealthy {
		healthy = colorize.Yellow("no (host)")
	}
	if !instance.Healthy {
		healthy = colorize.Red("no")
	}
	rejected := ""
	if instance.Rejection != nil {
		rejected = instance.Rejection.ID
		rejections[instance.Rejection.ID] = instance.Rejection.Desc
	}

	chosenStr := ""
	if chosen {
		chosenStr = colorize.Green("✓")
	} else {
		chosenStr = colorize.Gray("✘")
	}

	rtt := fmt.Sprintf("%.2fms", instance.NodeRttMs)
	if instance.NodeRttMs <= 60.0 {
		rtt = colorize.Green(rtt)
	} else if instance.NodeRttMs <= 120.0 {
		rtt = colorize.Yellow(rtt)
	} else {
		rtt = colorize.Red(rtt)
	}

	return []string{
		chosenStr, instance.ID, instance.State, instance.Region, healthy, strconv.Itoa(instance.Concurrency), rtt, rejected,
	}
}
