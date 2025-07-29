package scale

import (
	"testing"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
)

func Test_formatRegions(t *testing.T) {
	assert.Equal(t,
		formatRegions([]*fly.Machine{
			{Region: "fra"},
			{Region: "fra"},
			{Region: "fra"},
			{Region: "scl"},
			{Region: "scl"},
			{Region: "mia"},
		}),
		"fra(3),mia,scl(2)",
	)
}
