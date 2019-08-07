package presenters

import (
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
)

type ReleasePresenter struct {
	Releases []api.Release
	Release  *api.Release
}

func (p *ReleasePresenter) FieldNames() []string {
	return []string{"Version", "Reason", "Description", "User", "Date"}
}

func (p *ReleasePresenter) FieldMap() map[string]string {
	return map[string]string{
		"Version":     "Version",
		"Reason":      "Reason",
		"Description": "Description",
		"User":        "User",
		"Date":        "Date",
	}
}

func (p *ReleasePresenter) Records() []map[string]string {
	out := []map[string]string{}

	if p.Release != nil {
		p.Releases = append(p.Releases, *p.Release)
	}

	for _, release := range p.Releases {
		out = append(out, map[string]string{
			"Version":     fmt.Sprintf("v%d", release.Version),
			"Reason":      formatReleaseReason(release.Reason),
			"Description": formatReleaseDescription(release),
			"User":        release.User.Email,
			"Date":        formatRelativeTime(release.CreatedAt),
		})
	}

	return out
}

func formatReleaseReason(reason string) string {
	switch reason {
	case "change_image":
		return "Image"
	case "change_secrets":
		return "Secrets"
	case "change_code", "change_source": // nodeproxy
		return "Code Change"
	}
	return reason
}

func formatReleaseDescription(r api.Release) string {
	if r.Reason == "change_image" && strings.HasPrefix(r.Description, "deploy image ") {
		return r.Description[13:]
	}
	return r.Description
}
