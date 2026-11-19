package launchdarkly

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
)

const clientSideID string = "6557a71bbffb5f134b84b15c"

// evalURL is the client-side evaluation endpoint; a var so tests can point
// it at a local server.
var evalURL = "https://clientsdk.launchdarkly.com/sdk/evalx"

// WireGuardTokenModeFlag is the boolean flag that, when on for a user, makes
// the agent build that user's WireGuard tunnels through the token gateway
// regardless of the local wire_guard_token_mode config.
const WireGuardTokenModeFlag = "wireguard-token-mode"

type Client struct {
	ldContext  ldcontext.Context
	flags      map[string]FeatureFlag
	flagsMutex sync.Mutex
}

type contextKey struct{}

func NewContextWithClient(ctx context.Context, ldClient *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, ldClient)
}

func ClientFromContext(ctx context.Context) *Client {
	client := ctx.Value(contextKey{})
	if client == nil {
		return nil
	}

	return client.(*Client)
}

type UserInfo struct {
	OrganizationID string
	UserID         int
}

// newContext builds the evaluation context flyctl presents to LaunchDarkly:
// an organization and a user, each identified by its numeric id (0 when
// unknown). A known user is also keyed "user-<id>", which is how LaunchDarkly
// rules target individual users.
func newContext(userInfo UserInfo) (ldcontext.Context, error) {
	orgID := 0

	if userInfo.OrganizationID != "" {
		var err error

		orgID, err = strconv.Atoi(userInfo.OrganizationID)
		if err != nil {
			return ldcontext.Context{}, err
		}
	}

	orgContext := ldcontext.NewBuilder("flyctl").Anonymous(true).SetInt("id", orgID).Kind(ldcontext.Kind("organization")).Build()
	userKey := "flyctl"
	if userInfo.UserID != 0 {
		userKey = fmt.Sprintf("user-%d", userInfo.UserID)
	}
	userContext := ldcontext.NewBuilder(userKey).Anonymous(true).SetInt("id", userInfo.UserID).Kind(ldcontext.Kind("user")).Build()

	return ldcontext.NewMultiBuilder().Add(orgContext).Add(userContext).Build(), nil
}

func NewClient(ctx context.Context, userInfo UserInfo) (*Client, error) {
	logger := logger.MaybeFromContext(ctx)

	_, span := tracing.GetTracer().Start(ctx, "new_feature_flag_client")
	defer span.End()

	launchDarklyContext, err := newContext(userInfo)
	if err != nil {
		return nil, err
	}

	ldClient := &Client{ldContext: launchDarklyContext, flagsMutex: sync.Mutex{}}

	timeoutCtx, cancel := context.WithTimeoutCause(ctx, 10*time.Second, fmt.Errorf("initializing user feature flags: %w", context.DeadlineExceeded))
	defer cancel()
	// we don't really care if this errors or not, but it's good to at least try
	if err := ldClient.updateFeatureFlags(timeoutCtx); err != nil {
		logger.Debugf("update feature flags failed: %s", err)
	}

	go ldClient.monitor(ctx)

	return ldClient, nil
}

// NewUserClient returns a client that evaluates flags for the user with the
// given numeric id and no organization. Unlike NewClient it neither fetches
// anything nor starts the background poller: callers Refresh it whenever
// they want a current value, which suits long-lived processes like the agent
// that re-evaluate at well-defined moments.
func NewUserClient(userID int) *Client {
	// a context with no organization id can't fail to build
	ldContext, _ := newContext(UserInfo{UserID: userID})

	return &Client{ldContext: ldContext}
}

// Refresh re-fetches the flags from LaunchDarkly. Until a Refresh succeeds
// every flag reads as its default.
func (ldClient *Client) Refresh(ctx context.Context) error {
	return ldClient.updateFeatureFlags(ctx)
}

func NewServiceClient() (*Client, error) {
	ctx := context.Background()
	logger := logger.MaybeFromContext(ctx)

	_, span := tracing.GetTracer().Start(ctx, "new_flyctl_feature_flag_client")
	defer span.End()

	ldClient := &Client{
		ldContext: ldcontext.NewWithKind(ldcontext.Kind("service"), "flyctl"),
	}

	timeoutCtx, cancel := context.WithTimeoutCause(ctx, 10*time.Second, fmt.Errorf("initializing service feature flags: %w", context.DeadlineExceeded))
	defer cancel()
	// we don't really care if this errors or not, but it's good to at least try
	if err := ldClient.updateFeatureFlags(timeoutCtx); err != nil {
		logger.Debugf("update feature flags failed: %s", err)
	}

	go ldClient.monitor(ctx)

	return ldClient, nil
}

func (ldClient *Client) monitor(ctx context.Context) {
	logger := logger.MaybeFromContext(ctx)

	for {
		err := ldClient.updateFeatureFlags(ctx)
		if err != nil && logger != nil {
			logger.Debug("Failed to update feature flags from LaunchDarkly: ", err)
		}

		// the launchdarkly docs recommend polling every 30 seconds
		time.Sleep(30 * time.Second)
	}
}

func (ldClient *Client) GetFeatureFlagValue(key string, defaultValue any) any {
	_, span := tracing.GetTracer().Start(context.Background(), "get_feature_flag_value")
	defer span.End()

	span.SetAttributes(attribute.String("flag", key))

	ldClient.flagsMutex.Lock()
	defer ldClient.flagsMutex.Unlock()

	if flag, ok := ldClient.flags[key]; ok {
		return flag.Value
	}
	span.SetAttributes(attribute.Bool("default_flag", true))

	return defaultValue
}

type FeatureFlag struct {
	FlagVersion int  `json:"flagVersion"`
	TrackEvents bool `json:"trackEvents"`
	Value       any  `json:"value"`
	Version     int  `json:"version"`
	Variation   int  `json:"variation"`
}

func (ldClient *Client) updateFeatureFlags(ctx context.Context) error {
	_, span := tracing.GetTracer().Start(ctx, "update_feature_flags")
	defer span.End()

	ldContextJSON := ldClient.ldContext.JSONString()
	ldContextB64 := base64.URLEncoding.EncodeToString([]byte(ldContextJSON))

	url := fmt.Sprintf("%s/%s/contexts/%s", evalURL, clientSideID, ldContextB64)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		tracing.RecordErrorEvent(ctx, span, err)

		return err
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		tracing.RecordErrorEvent(ctx, span, err)

		return err
	}
	defer response.Body.Close()

	var flags map[string]FeatureFlag
	if err := json.NewDecoder(response.Body).Decode(&flags); err != nil {
		tracing.RecordErrorEvent(ctx, span, err)

		return err
	}

	if flags == nil {
		span.AddEvent("no flags returned")

		return nil
	}

	flagAttributes := lo.MapToSlice(flags, func(flag string, flagInfo FeatureFlag) *attribute.KeyValue {
		switch flagInfo.Value.(type) {
		case bool:
			attr := attribute.Bool(flag, flagInfo.Value.(bool))

			return &attr
		case string:
			attr := attribute.String(flag, flagInfo.Value.(string))

			return &attr
		case float64:
			attr := attribute.Float64(flag, flagInfo.Value.(float64))

			return &attr
		default:
			span.AddEvent(fmt.Sprintf("unaccounted for flag type: %s", reflect.TypeOf(flagInfo.Value)))

			return nil
		}
	})

	for _, flagAttribute := range flagAttributes {
		if flagAttribute != nil {
			span.SetAttributes(*flagAttribute)
		}
	}

	ldClient.flagsMutex.Lock()
	ldClient.flags = flags
	ldClient.flagsMutex.Unlock()

	return nil
}

func (ldClient *Client) ManagedPostgresEnabled() bool {
	choice := ldClient.getLaunchPostgresChoiceFlag()

	return choice == "mpg" || choice == "both"
}

func (ldClient *Client) UnmanagedPostgresEnabled() bool {
	choice := ldClient.getLaunchPostgresChoiceFlag()

	return choice == "unmanaged-pg" || choice == "both"
}

func (ldClient *Client) getLaunchPostgresChoiceFlag() string {
	return ldClient.GetFeatureFlagValue("launch-postgres-choice", "unmanaged-pg").(string)
}

func (ldClient *Client) getManagedBuilderEnabled() bool {
	return ldClient.GetFeatureFlagValue("managed-builder", false).(bool)
}

func (ldClient *Client) ManagedBuilderEnabled() bool {
	return ldClient.getManagedBuilderEnabled()
}

func (ldClient *Client) UseZstdEnabled() bool {
	return ldClient.GetFeatureFlagValue("use-zstd-for-docker-images", false).(bool)
}

func (ldClient *Client) GetCompressionStrength() any {
	return ldClient.GetFeatureFlagValue("flyctl-compression-strength", 7)
}

// WireGuardTokenModeEnabled reports whether WireGuardTokenModeFlag is on.
// Anything but a boolean true (unset, false, a mistyped flag) counts as off.
func (ldClient *Client) WireGuardTokenModeEnabled() bool {
	on, _ := ldClient.GetFeatureFlagValue(WireGuardTokenModeFlag, false).(bool)

	return on
}
