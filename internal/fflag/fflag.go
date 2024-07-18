package fflag

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/superfly/flyctl/internal/tracing"
)

const clientSideID string = "6557a71bbffb5f134b84b15c"

type FeatureFlagClient struct {
	ldContext  ldcontext.Context
	flags      map[string]FeatureFlag
	flagsMutex sync.Mutex
}

func NewContextWithClient(ctx context.Context, ffClient *FeatureFlagClient) context.Context {
	return context.WithValue(ctx, "featureFlagClient", ffClient)
}

func ClientFromContext(ctx context.Context) *FeatureFlagClient {
	client := ctx.Value("featureFlagClient")
	if client == nil {
		return nil
	}
	return client.(*FeatureFlagClient)
}

type UserInfo struct {
	OrganizationID string
	UserID         int
}

func NewClient(ctx context.Context, userInfo UserInfo) (*FeatureFlagClient, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "new_feature_flag_client")
	defer span.End()

	orgID, err := strconv.Atoi(userInfo.OrganizationID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	orgContext := ldcontext.NewBuilder("flyctl").Anonymous(true).SetInt("id", orgID).Kind(ldcontext.Kind("organization")).Build()
	userContext := ldcontext.NewBuilder("flyctl").Anonymous(true).SetInt("id", userInfo.UserID).Kind(ldcontext.Kind("user")).Build()

	launchDarklyContext := ldcontext.NewMultiBuilder().Add(orgContext).Add(userContext).Build()

	ffClient := &FeatureFlagClient{
		ldContext:  launchDarklyContext,
		flagsMutex: sync.Mutex{},
	}

	go func() {
		for {
			flags, err := ffClient.updateFeatureFlags()
			if err != nil {
				return
			}

			ffClient.flagsMutex.Lock()
			ffClient.flags = flags
			ffClient.flagsMutex.Unlock()

			// the launchdarkly docs recommend polling every 30 seconds
			time.Sleep(30 * time.Second)
		}
	}()

	return ffClient, nil
}

func (ffClient *FeatureFlagClient) GetFeatureFlagValue(key string, defaultValue interface{}) interface{} {
	ffClient.flagsMutex.Lock()
	defer ffClient.flagsMutex.Unlock()

	if flag, ok := ffClient.flags[key]; ok {
		return flag.Value
	}
	return defaultValue

}

type FeatureFlag struct {
	FlagVersion int         `json:"flagVersion"`
	TrackEvents bool        `json:"trackEvents"`
	Value       interface{} `json:"value"`
	Version     int         `json:"version"`
	Variation   int         `json:"variation"`
}

func (ffClient *FeatureFlagClient) updateFeatureFlags() (map[string]FeatureFlag, error) {
	ldContextJSON := ffClient.ldContext.JSONString()
	ldContextB64 := base64.URLEncoding.EncodeToString([]byte(ldContextJSON))

	request, err :=
		http.NewRequest("GET", fmt.Sprintf("https://clientsdk.launchdarkly.com/sdk/evalx/%s/contexts/%s", clientSideID, ldContextB64), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	resp, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var flags map[string]FeatureFlag
	err = json.Unmarshal(resp, &flags)
	if err != nil {
		return nil, err
	}

	return flags, nil
}
