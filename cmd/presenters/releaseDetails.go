package presenters

import (
	"fmt"

	"github.com/superfly/flyctl/api"
)

type ReleaseDetails struct {
	Release api.Release
}

func (p *ReleaseDetails) FieldNames() []string {
	return []string{"Version", "Reason", "Description", "Status", "Status Description", "User", "Date"}
}

func (p *ReleaseDetails) Records() []map[string]string {
	out := []map[string]string{}

	release := p.Release

	out = append(out, map[string]string{
		"Version":            fmt.Sprintf("v%d", release.Version),
		"Reason":             formatReleaseReason(release.Reason),
		"Description":        release.Description,
		"Stable":             fmt.Sprintf("%t", release.Stable),
		"User":               release.User.Email,
		"Date":               formatRelativeTime(release.CreatedAt),
		"Status":             release.Status,
		"Status Description": release.Deployment.Description,
	})

	return out
}
