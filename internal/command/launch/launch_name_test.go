package launch

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestAppNameIsDerived(t *testing.T) {
	assert.True(t, appNameIsDerived(&launchPlanSource{appNameSource: appNameDerivedFromDir}))
	assert.False(t, appNameIsDerived(&launchPlanSource{appNameSource: "specified on the command line"}))
	assert.False(t, appNameIsDerived(&launchPlanSource{appNameSource: "from your fly.toml"}))
	assert.False(t, appNameIsDerived(nil))
}

func flapsError(status int, body string) error {
	return &flaps.FlapsError{
		OriginalError:      errors.New(body),
		ResponseStatusCode: status,
		ResponseBody:       []byte(body),
	}
}

// createApp picks a free name only when the API says the name is taken, going
// by its status code rather than the message: a retry that gave up can carry
// the "taken" text from an attempt that may have created the app, and the race
// with another create returns name_taken with a generic message.
func TestCreateAppRenamesOnlyWhenTheNameIsTaken(t *testing.T) {
	for _, tc := range []struct {
		name       string
		createErr  error
		nameSource string
		wantRename bool
	}{
		{
			name:       "name_taken status",
			createErr:  flapsError(http.StatusUnprocessableEntity, `{"error":"uniqueness constraint violated","status":"name_taken"}`),
			nameSource: appNameDerivedFromDir,
			wantRename: true,
		},
		{
			name:       "legacy message without a status",
			createErr:  flapsError(http.StatusUnprocessableEntity, `{"error":"Validation failed: Name has already been taken"}`),
			nameSource: appNameDerivedFromDir,
			wantRename: true,
		},
		{
			name:       "other failure carrying the taken message",
			createErr:  flapsError(http.StatusServiceUnavailable, `{"error":"retry gave up after: Validation failed: Name has already been taken","status":"insufficient_capacity"}`),
			nameSource: appNameDerivedFromDir,
		},
		{
			name:       "name the user chose",
			createErr:  flapsError(http.StatusUnprocessableEntity, `{"error":"uniqueness constraint violated","status":"name_taken"}`),
			nameSource: "specified on the command line",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var created []string
			ios, _, _, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), ios)
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{})
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				CreateAppFunc: func(_ context.Context, req flaps.CreateAppRequest) (*flaps.App, error) {
					created = append(created, req.Name)
					if len(created) == 1 {
						return nil, tc.createErr
					}
					return &flaps.App{Name: req.Name}, nil
				},
				AppNameAvailableFunc: func(context.Context, string) (bool, error) { return true, nil },
				WaitForAppFunc:       func(context.Context, string) error { return nil },
			})

			state := &launchState{
				LaunchManifest: LaunchManifest{
					Plan:       &plan.LaunchPlan{AppName: "my-app", OrgSlug: "personal"},
					PlanSource: &launchPlanSource{appNameSource: tc.nameSource},
				},
				planBuildCache: planBuildCache{appConfig: appconfig.NewConfig()},
				cache:          map[string]any{},
			}

			app, err := state.createApp(ctx)

			if !tc.wantRename {
				assert.ErrorIs(t, err, tc.createErr)
				assert.Equal(t, []string{"my-app"}, created)
				assert.Equal(t, "my-app", state.Plan.AppName)
				return
			}
			require.NoError(t, err)
			require.Len(t, created, 2)
			assert.Regexp(t, `^my-app-[a-z]+-[a-z]+-\d+$`, created[1])
			assert.Equal(t, created[1], app.Name)
			assert.Equal(t, created[1], state.Plan.AppName)
			assert.Equal(t, created[1], state.appConfig.AppName)
		})
	}
}
