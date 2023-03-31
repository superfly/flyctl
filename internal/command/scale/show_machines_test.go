package scale

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/api"
)

func Test_formatRegions(t *testing.T) {
	assert.Equal(t,
		formatRegions([]*api.Machine{
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
