package presenters

import (
	"github.com/superfly/flyctl/api"
)

type ImageVersion struct {
	ImageDetails api.ImageVersion
}

func (p *ImageVersion) APIStruct() interface{} {
	return p.ImageDetails
}

func (p *ImageVersion) FieldNames() []string {
	return []string{"Repository", "Tag", "Version", "Digest"}
}

func (p *ImageVersion) Records() []map[string]string {
	out := []map[string]string{}

	info := map[string]string{
		"Repository": p.ImageDetails.Repository,
		"Tag":        p.ImageDetails.Tag,
		"Version":    p.ImageDetails.Version,
		"Digest":     p.ImageDetails.Digest,
	}

	out = append(out, info)

	return out
}
