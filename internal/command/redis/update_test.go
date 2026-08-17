package redis

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/prompt"
)

type captureGenqClient struct {
	variables map[string]any
	requests  int
	opNames   []string
}

func (c *captureGenqClient) MakeRequest(_ context.Context, req *genq.Request, resp *genq.Response) error {
	c.requests++
	c.opNames = append(c.opNames, req.OpName)
	encoded, err := json.Marshal(req.Variables)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(encoded, &c.variables); err != nil {
		return err
	}
	resp.Data = map[string]any{}
	return nil
}

// confirmStub returns a canned answer and records whether it was called.
func confirmStub(answer bool, err error, called *bool) func() (bool, error) {
	return func() (bool, error) {
		*called = true
		return answer, err
	}
}

func TestResolveProdPack(t *testing.T) {
	nonInteractive := prompt.NonInteractiveError("prompt: non interactive")

	tests := []struct {
		name         string
		enableFlag   bool
		disableFlag  bool
		stored       any
		legacy       bool
		answer       bool
		answerErr    error
		want         *bool
		wantErr      bool
		wantPrompted bool
	}{
		{
			name:   "no flags, key absent: no decision, no prompt",
			stored: nil,
			want:   nil,
		},
		{
			name:   "no flags, stored false: no decision, no prompt",
			stored: false,
			want:   nil,
		},
		{
			name:         "no flags, stored true, user declines disable: no decision",
			stored:       true,
			answer:       false,
			want:         nil,
			wantPrompted: true,
		},
		{
			name:         "no flags, stored true, user confirms disable: explicit false",
			stored:       true,
			answer:       true,
			want:         boolPtr(false),
			wantPrompted: true,
		},
		{
			name:         "no flags, stored true, non-interactive: no decision instead of disable",
			stored:       true,
			answerErr:    nonInteractive,
			want:         nil,
			wantPrompted: true,
		},
		{
			name:         "no flags, stored true, prompt error propagates",
			stored:       true,
			answerErr:    errors.New("boom"),
			wantErr:      true,
			wantPrompted: true,
		},
		{
			name:       "enable flag: explicit true, no prompt",
			enableFlag: true,
			stored:     nil,
			want:       boolPtr(true),
		},
		{
			name:        "disable flag: explicit false even when key absent",
			disableFlag: true,
			stored:      nil,
			want:        boolPtr(false),
		},
		{
			name:        "both flags: error",
			enableFlag:  true,
			disableFlag: true,
			wantErr:     true,
		},
		{
			name:       "legacy plan, enable flag: error",
			enableFlag: true,
			legacy:     true,
			wantErr:    true,
		},
		{
			name:        "legacy plan, disable flag: explicit false",
			disableFlag: true,
			legacy:      true,
			want:        boolPtr(false),
		},
		{
			name:   "legacy plan, stored true: no decision, no prompt",
			stored: true,
			legacy: true,
			want:   nil,
		},
		{
			name:   "stored non-bool garbage: treated as unknown, no prompt",
			stored: "yes",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompted := false
			got, err := resolveProdPack(tt.enableFlag, tt.disableFlag, tt.stored, tt.legacy, confirmStub(tt.answer, tt.answerErr, &prompted))

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPrompted, prompted, "prompt invocation")

			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}

func TestStripProdPack(t *testing.T) {
	options := map[string]any{"prod_pack": true, "eviction": true}
	stripProdPack(options)
	_, present := options["prod_pack"]
	assert.False(t, present, "prod_pack must be omitted from the legacy options blob")
	assert.Equal(t, true, options["eviction"], "other options untouched")
}

func TestValidateProdPackPlanChange(t *testing.T) {
	tests := []struct {
		name      string
		decision  *bool
		current   string
		selected  string
		expectErr bool
	}{
		{name: "explicit decision with same plan", decision: boolPtr(false), current: "plan", selected: "plan", expectErr: true},
		{name: "explicit decision with changed plan", decision: boolPtr(true), current: "plan", selected: "other"},
		{name: "no decision with same plan", current: "plan", selected: "plan"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProdPackPlanChange(tt.decision, tt.current, tt.selected)
			if tt.expectErr {
				require.EqualError(t, err, "ProdPack can only be changed together with a plan change; pick a different plan or drop the flag")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdateRedisAddOnWire(t *testing.T) {
	tests := []struct {
		name         string
		enable       bool
		disable      bool
		stored       any
		wantProdPack any
	}{
		{
			name:         "no intent omits option key and sends null typed input",
			stored:       true,
			wantProdPack: nil,
		},
		{
			name:         "explicit disable uses typed input and omits option key",
			disable:      true,
			wantProdPack: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This mirrors production wiring: resolve intent, sanitize the
			// legacy options blob, then pass intent through the typed operation.
			decision, err := resolveProdPack(tt.enable, tt.disable, tt.stored, false, func() (bool, error) {
				return false, nil
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantProdPack, func() any {
				if decision == nil {
					return nil
				}
				return *decision
			}())

			options := map[string]any{"prod_pack": true, "eviction": true}
			stripProdPack(options)
			client := &captureGenqClient{}
			_, err = gql.UpdateRedisAddOn(context.Background(), client, "addon", "plan", []string{"ord"}, options, map[string]any{}, decision)
			require.NoError(t, err)
			assert.Equal(t, 1, client.requests)
			assert.Equal(t, []string{"UpdateRedisAddOn"}, client.opNames)
			assert.Equal(t, tt.wantProdPack, client.variables["prodPack"])
			options, ok := client.variables["options"].(map[string]any)
			require.True(t, ok)
			_, present := options["prod_pack"]
			assert.False(t, present)
		})
	}
}

func TestSamePlanProdPackDecisionRejectsBeforeGraphQLRequest(t *testing.T) {
	client := &captureGenqClient{}
	err := validateProdPackPlanChange(boolPtr(false), "plan", "plan")

	require.EqualError(t, err, "ProdPack can only be changed together with a plan change; pick a different plan or drop the flag")
	assert.Zero(t, client.requests, "same-plan intent must stop before the GraphQL mutation")
}
