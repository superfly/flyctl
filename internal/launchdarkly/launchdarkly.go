package launchdarkly

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
)

const clientSideID string = "6557a71bbffb5f134b84b15c"

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

func NewClient(ctx context.Context, userInfo UserInfo) (*Client, error) {
	_, span := tracing.GetTracer().Start(ctx, "new_feature_flag_client")
	defer span.End()

	orgID := 0

	if userInfo.OrganizationID != "" {
		var err error

		orgID, err = strconv.Atoi(userInfo.OrganizationID)
		if err != nil {
			return nil, err
		}
	}

	orgContext := ldcontext.NewBuilder("flyctl").Anonymous(true).SetInt("id", orgID).Kind(ldcontext.Kind("organization")).Build()
	userContext := ldcontext.NewBuilder("flyctl").Anonymous(true).SetInt("id", userInfo.UserID).Kind(ldcontext.Kind("user")).Build()

	launchDarklyContext := ldcontext.NewMultiBuilder().Add(orgContext).Add(userContext).Build()

	ldClient := &Client{ldContext: launchDarklyContext, flagsMutex: sync.Mutex{}}

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// we don't really care if this errors or not, but it's good to at least try
	_ = ldClient.updateFeatureFlags(timeoutCtx)

	go ldClient.monitor(ctx)
	return ldClient, nil
}

func NewServiceClient() (*Client, error) {
	ctx := context.Background()
	_, span := tracing.GetTracer().Start(ctx, "new_flyctl_feature_flag_client")
	defer span.End()

	ldClient := &Client{ldContext: ldcontext.NewWithKind(ldcontext.Kind("service"), "flyctl"), flagsMutex: sync.Mutex{}}

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// we don't really care if this errors or not, but it's good to at least try
	_ = ldClient.updateFeatureFlags(timeoutCtx)

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

	flags, err := fetchFlags(ctx, func(ctx context.Context) (map[string]FeatureFlag, error) {
		var flags map[string]FeatureFlag

		ldContextJSON := ldClient.ldContext.JSONString()
		ldContextB64 := base64.URLEncoding.EncodeToString([]byte(ldContextJSON))

		url := fmt.Sprintf("https://clientsdk.launchdarkly.com/sdk/evalx/%s/contexts/%s", clientSideID, ldContextB64)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}

		response, err := http.DefaultClient.Do(req)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}
		defer response.Body.Close()

		if err := json.NewDecoder(response.Body).Decode(&flags); err != nil {
			span.RecordError(err)
			return nil, err
		}
		return flags, nil
	})

	if err != nil {
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

const (
	cacheKey = "launchdarkly-flags"
	ttl      = time.Hour
	refresh  = 5 * time.Minute
)

func fetchFlags(ctx context.Context,
	fetchFn func(context.Context) (map[string]FeatureFlag, error),
) (map[string]FeatureFlag, error) {
	c := cache.FromContext(ctx)
	if c == nil {
		return nil, errors.New("cache not present in context")
	}

	obj, err := c.Fetch(cacheKey, ttl, refresh,
		func() (any, error) {
			return fetchFn(ctx)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("fetching LaunchDarkly flags: %w", err)
	}

	flags, ok := obj.(map[string]FeatureFlag)
	if !ok {
		return nil, fmt.Errorf("unexpected cache payload type: %T", obj)
	}
	return flags, nil
}
