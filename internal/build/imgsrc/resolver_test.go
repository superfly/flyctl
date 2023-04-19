package imgsrc

import (
	"context"
	"net/http"
	"testing"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
)

func TestHeartbeat(t *testing.T) {
	dc, err := client.NewClientWithOpts();
	assert.NoError(t, err)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "", http.NoBody)
	assert.NoError(t, err)

	err = heartbeat(dc, req)
	assert.Error(t, err)
}
