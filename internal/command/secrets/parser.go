package secrets

import (
	"io"

	"github.com/hashicorp/go-envparse"
)

func parseSecrets(reader io.Reader) (map[string]string, error) {
	return envparse.Parse(reader)
}
