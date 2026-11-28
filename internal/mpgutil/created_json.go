package mpgutil

import (
	"fmt"
	"io"

	"github.com/superfly/flyctl/internal/render"
)

// RenderCreatedJSON preserves the created resource identity if output fails.
func RenderCreatedJSON(out io.Writer, created string, result any) error {
	if err := render.JSON(out, result); err != nil {
		return fmt.Errorf("%s was created, but its result could not be displayed: %w", created, err)
	}
	return nil
}
