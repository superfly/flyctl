package flapsutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/fly-go/flaps"
)

func TestHasErrorStatusCode(t *testing.T) {
	placementErr := &flaps.FlapsError{
		OriginalError: errors.New("volume placement failed"),
		ResponseBody:  []byte(`{"status":"volume_placement_capacity"}`),
	}
	otherFlapsErr := &flaps.FlapsError{
		OriginalError: errors.New("another failure"),
		ResponseBody:  []byte(`{"status":"unknown"}`),
	}

	assert.True(t, HasErrorStatusCode(placementErr, VolumePlacementCapacityStatus))
	assert.True(t, HasErrorStatusCode(fmt.Errorf("launch failed: %w", placementErr), VolumePlacementCapacityStatus))
	assert.True(t, HasErrorStatusCode(errors.Join(otherFlapsErr, placementErr), VolumePlacementCapacityStatus))
	assert.False(t, HasErrorStatusCode(otherFlapsErr, VolumePlacementCapacityStatus))
	assert.False(t, HasErrorStatusCode(errors.New("not a Flaps error"), VolumePlacementCapacityStatus))
	assert.False(t, HasErrorStatusCode(nil, VolumePlacementCapacityStatus))
}
