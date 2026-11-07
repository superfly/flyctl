package launch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAppNameTakenErr(t *testing.T) {
	assert.True(t, isAppNameTakenErr(errors.New("Validation failed: Name has already been taken")))
	assert.True(t, isAppNameTakenErr(errors.New("failed to create app: name HAS ALREADY BEEN TAKEN")))
	assert.False(t, isAppNameTakenErr(errors.New("payment method required")))
	assert.False(t, isAppNameTakenErr(nil))
}

func TestAppNameIsDerived(t *testing.T) {
	assert.True(t, appNameIsDerived(&launchPlanSource{appNameSource: appNameDerivedFromDir}))
	assert.False(t, appNameIsDerived(&launchPlanSource{appNameSource: "specified on the command line"}))
	assert.False(t, appNameIsDerived(&launchPlanSource{appNameSource: "from your fly.toml"}))
	assert.False(t, appNameIsDerived(nil))
}
