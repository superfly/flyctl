package presenters

import (
	"github.com/superfly/flyctl/api"
)

type ImageDetails struct {
	ImageDetails    api.ImageVersion
	TrackingEnabled bool
}

func (p *ImageDetails) APIStruct() interface{} {
	return p.ImageDetails
}

func (p *ImageDetails) FieldNames() []string {

	return []string{"Registry", "Repository", "Tag", "Version", "Digest"}
}

func (p *ImageDetails) Records() []map[string]string {
	out := []map[string]string{}

	info := map[string]string{
		"Registry":   p.ImageDetails.Registry,
		"Repository": p.ImageDetails.Repository,
		"Tag":        p.ImageDetails.Tag,
		"Version":    p.ImageDetails.Version,
		"Digest":     p.ImageDetails.Digest,
	}

	if info["Version"] == "" {
		if p.TrackingEnabled {
			info["Version"] = "Not specified"
		} else {
			info["Version"] = "N/A"
		}

	}

	out = append(out, info)

	return out
}
