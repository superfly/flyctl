package secrets

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// Maximum number of machines to check for deployment status
const maxMachinesToCheck = 100

// SecretStatus represents the deployment status of a secret
type SecretStatus string

const (
	StatusDeployed          SecretStatus = "Deployed"
	StatusStaged            SecretStatus = "Staged"
	StatusPartiallyDeployed SecretStatus = "Partial"
	StatusUnknown           SecretStatus = "Unknown"
)

// SecretWithStatus extends fly.AppSecret with deployment status
type SecretWithStatus struct {
	Name   string       `json:"name"`
	Digest string       `json:"digest"`
	Status SecretStatus `json:"status"`
}

func newList() (cmd *cobra.Command) {
	const (
		long = `List the secrets available to the application. It shows each secret's
name, a digest of its value and the deployment status across machines. The
actual value of the secret is only available to the application.

Secrets that need deployment are prefixed with an indicator:
  *  Staged secret (not deployed to any machines)
  !  Partial deployment (deployed to some but not all machines)

Deployment status:
  Deployed     - Secret is deployed to all machines (secret updated_at <= machine release created_at)
  Staged       - Secret is staged but not deployed to any machines
  Partial      - Secret is deployed to some but not all machines (rolling deployment in progress)
  Unknown      - Status cannot be determined (missing timestamps, too many machines, or API error)`
		short = `List application secret names, digests and deployment status`
		usage = "list [flags]"
	)

	cmd = command.New(usage, short, long, runList, command.RequireSession, command.RequireAppName)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)

	return cmd
}

func runList(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	rows, secretsWithStatus, stagedCount, partialCount, statusAvailable, err := buildSecretRows(ctx, flapsClient, uiexClient, appName)
	if err != nil {
		return err
	}

	headers := []string{"Name", "Digest"}
	if statusAvailable {
		headers = append(headers, "Status")
	}

	if cfg.JSONOutput {
		if statusAvailable {
			return render.JSON(out, secretsWithStatus)
		}
		// When status is not available, build JSON from secrets
		type secretBasic struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		}
		basicSecrets := make([]secretBasic, len(rows))
		for i, row := range rows {
			basicSecrets[i] = secretBasic{Name: row[0], Digest: row[1]}
		}
		return render.JSON(out, basicSecrets)
	}

	if err := render.Table(out, "", rows, headers...); err != nil {
		return err
	}

	if stagedCount > 0 || partialCount > 0 {
		printDeploymentSummary(out, stagedCount, partialCount)
	}

	return nil
}

func buildSecretRows(ctx context.Context, flapsClient flapsutil.FlapsClient, uiexClient uiexutil.Client, appName string) ([][]string, []SecretWithStatus, int, int, bool, error) {
	secrets, err := appsecrets.List(ctx, flapsClient, appName)
	if err != nil {
		return nil, nil, 0, 0, false, err
	}

	// Get machines to compute deployment status
	machines, _, err := flapsClient.ListFlyAppsMachines(ctx, appName)
	if err != nil {
		// If we can't get machines, show secrets without status
		machines = nil
	}

	// Filter out destroyed/destroying machines
	relevantMachines := filterRelevantMachines(machines)

	// Skip status computation if too many machines - just show name and digest
	if len(relevantMachines) > maxMachinesToCheck {
		return buildRowsWithoutStatus(secrets), nil, 0, 0, false, nil
	}

	// Fetch release timestamps
	releaseTimestamps := fetchReleaseTimestamps(ctx, uiexClient, appName, collectReleaseVersions(relevantMachines))

	// Pre-compute version counts
	versionCounts := buildVersionCounts(relevantMachines, releaseTimestamps)

	// Compute per-secret deployment status and count staged and partially deployed secrets
	secretsWithStatus := make([]SecretWithStatus, 0, len(secrets))
	stagedCount := 0
	partialCount := 0
	for _, secret := range secrets {
		status := computeSecretStatus(secret, versionCounts)
		if status == StatusStaged {
			stagedCount++
		}
		if status == StatusPartiallyDeployed {
			partialCount++
		}
		secretsWithStatus = append(secretsWithStatus, SecretWithStatus{
			Name:   secret.Name,
			Digest: secret.Digest,
			Status: status,
		})
	}

	var rows [][]string
	for _, secret := range secretsWithStatus {
		var prefix string
		switch secret.Status {
		case StatusStaged:
			prefix = "* "
		case StatusPartiallyDeployed:
			prefix = "! "
		}

		rows = append(rows, []string{
			prefix + secret.Name,
			secret.Digest,
			string(secret.Status),
		})
	}

	return rows, secretsWithStatus, stagedCount, partialCount, true, nil
}

func buildRowsWithoutStatus(secrets []fly.AppSecret) [][]string {
	rows := make([][]string, 0, len(secrets))
	for _, secret := range secrets {
		rows = append(rows, []string{
			secret.Name,
			secret.Digest,
		})
	}
	return rows
}

func printDeploymentSummary(out io.Writer, stagedCount, partialCount int) {
	if stagedCount == 0 && partialCount == 0 {
		return
	}

	if stagedCount > 0 && partialCount > 0 {
		// Both staged and partial - use bullet list format
		fmt.Fprintf(out, "Some secrets need to be deployed:\n")

		stagedPlural := "secrets"
		if stagedCount == 1 {
			stagedPlural = "secret"
		}
		fmt.Fprintf(out, "  * %d %s staged (not yet deployed)\n", stagedCount, stagedPlural)

		partialPlural := "secrets"
		if partialCount == 1 {
			partialPlural = "secret"
		}
		fmt.Fprintf(out, "  ! %d %s partially deployed (deployed to some machines)\n", partialCount, partialPlural)

		fmt.Fprintf(out, "\nDeploy with `fly secrets deploy` to sync all machines.\n")
	} else if stagedCount > 0 {
		// Only staged secrets
		verb := "are"
		plural := "secrets"
		if stagedCount == 1 {
			verb = "is"
			plural = "secret"
		}
		fmt.Fprintf(out, "There %s %d %s not deployed. Deploy with `fly secrets deploy` to make them available.\n", verb, stagedCount, plural)
	} else if partialCount > 0 {
		// Only partial secrets
		verb := "are"
		plural := "secrets"
		if partialCount == 1 {
			verb = "is"
			plural = "secret"
		}
		fmt.Fprintf(out, "There %s %d %s partially deployed. This can happen during rolling deployments or if some machines failed to update. Deploy with `fly secrets deploy` to ensure all machines have the latest configuration.\n", verb, partialCount, plural)
	}
}

func filterRelevantMachines(machines []*fly.Machine) []*fly.Machine {
	if machines == nil {
		return nil
	}

	relevant := make([]*fly.Machine, 0, len(machines))
	for _, m := range machines {
		if m.State != "destroyed" && m.State != "destroying" {
			relevant = append(relevant, m)
		}
	}
	return relevant
}

func getMachineReleaseVersion(m *fly.Machine) string {
	if m == nil || m.Config == nil || m.Config.Metadata == nil {
		return ""
	}
	return m.Config.Metadata[fly.MachineConfigMetadataKeyFlyReleaseVersion]
}

type versionInfo struct {
	createdAt    time.Time
	machineCount int
}

type versionCounts struct {
	totalMachines int
	versions      map[string]versionInfo
}

func buildVersionCounts(machines []*fly.Machine, releaseTimestamps map[string]time.Time) versionCounts {
	result := versionCounts{
		versions: make(map[string]versionInfo),
	}

	if machines == nil {
		return result
	}

	for _, machine := range machines {
		result.totalMachines++
		version := getMachineReleaseVersion(machine)
		createdAt, ok := releaseTimestamps[version]
		if !ok {
			continue
		}

		if info, exists := result.versions[version]; exists {
			info.machineCount++
			result.versions[version] = info
		} else {
			result.versions[version] = versionInfo{
				createdAt:    createdAt,
				machineCount: 1,
			}
		}
	}

	return result
}

func computeSecretStatus(secret fly.AppSecret, vc versionCounts) SecretStatus {
	if vc.totalMachines == 0 {
		return StatusStaged
	}

	// Parse the UpdatedAt timestamp string
	secretUpdatedAt := parseTimestamp(secret.UpdatedAt)
	if secretUpdatedAt == nil {
		return StatusUnknown
	}

	// Loop through unique versions instead of all machines
	machinesWithSecret := 0
	for _, info := range vc.versions {
		if !secretUpdatedAt.After(info.createdAt) {
			machinesWithSecret += info.machineCount
		}
	}

	switch {
	case machinesWithSecret == 0:
		return StatusStaged
	case machinesWithSecret == vc.totalMachines:
		return StatusDeployed
	default:
		return StatusPartiallyDeployed
	}
}

func collectReleaseVersions(machines []*fly.Machine) []string {
	if machines == nil {
		return nil
	}

	versions := make([]string, 0, len(machines))
	for _, machine := range machines {
		version := getMachineReleaseVersion(machine)
		if version != "" {
			versions = append(versions, version)
		}
	}
	return versions
}

func fetchReleaseTimestamps(
	ctx context.Context,
	uiexClient uiexutil.Client,
	appName string,
	releaseVersions []string,
) map[string]time.Time {
	timestamps := make(map[string]time.Time)

	if uiexClient == nil || len(releaseVersions) == 0 {
		return timestamps
	}

	uniqueVersions := make(map[string]struct{}, len(releaseVersions))
	for _, v := range releaseVersions {
		if v == "" {
			continue
		}
		uniqueVersions[v] = struct{}{}
	}

	if len(uniqueVersions) == 0 {
		return timestamps
	}

	limit := len(uniqueVersions) * 2
	if limit < 20 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	releases, err := uiexClient.ListReleases(ctx, appName, limit)
	if err != nil {
		return timestamps
	}

	for _, release := range releases {
		versionKey := strconv.Itoa(release.Version)
		if _, ok := uniqueVersions[versionKey]; ok {
			timestamps[versionKey] = release.CreatedAt
		}
	}

	return timestamps
}

// parseTimestamp attempts to parse a timestamp string into a time.Time.
// Returns nil if the string pointer is nil, empty, or cannot be parsed.
func parseTimestamp(value *string) *time.Time {
	if value == nil || *value == "" {
		return nil
	}

	// Try RFC3339 (with or without timezone)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.999999",
	}

	for _, layout := range layouts {
		if ts, err := time.Parse(layout, *value); err == nil {
			return &ts
		}
	}

	return nil
}
