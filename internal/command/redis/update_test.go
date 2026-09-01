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

// confirmStub returns a canned answer, records whether it was called and
// records the current-state argument the caller phrased the prompt from.
func confirmStub(answer bool, err error, called *bool, currentlyOn *bool) func(bool) (bool, error) {
	return func(on bool) (bool, error) {
		*called = true
		*currentlyOn = on

		return answer, err
	}
}

func TestResolveProdPack(t *testing.T) {
	nonInteractive := prompt.NonInteractiveError("prompt: non interactive")

	tests := []struct {
		name           string
		enableFlag     bool
		disableFlag    bool
		stored         any
		legacy         bool
		answer         bool
		answerErr      error
		want           *bool
		wantErr        bool
		wantPrompted   bool
		wantPromptedOn bool
	}{
		{
			name:         "no flags, key absent, user confirms enable: explicit true",
			stored:       nil,
			answer:       true,
			want:         boolPtr(true),
			wantPrompted: true,
		},
		{
			name:         "no flags, key absent, user declines enable: no decision",
			stored:       nil,
			answer:       false,
			want:         nil,
			wantPrompted: true,
		},
		{
			name:         "no flags, stored false, user confirms enable: explicit true",
			stored:       false,
			answer:       true,
			want:         boolPtr(true),
			wantPrompted: true,
		},
		{
			name:         "no flags, stored false, user declines enable: no decision",
			stored:       false,
			answer:       false,
			want:         nil,
			wantPrompted: true,
		},
		{
			name:           "no flags, stored true, user declines disable: no decision",
			stored:         true,
			answer:         false,
			want:           nil,
			wantPrompted:   true,
			wantPromptedOn: true,
		},
		{
			name:           "no flags, stored true, user confirms disable: explicit false",
			stored:         true,
			answer:         true,
			want:           boolPtr(false),
			wantPrompted:   true,
			wantPromptedOn: true,
		},
		{
			name:           "no flags, stored true, non-interactive: no decision instead of disable",
			stored:         true,
			answerErr:      nonInteractive,
			want:           nil,
			wantPrompted:   true,
			wantPromptedOn: true,
		},
		{
			name:         "no flags, key absent, non-interactive: no decision instead of enable",
			stored:       nil,
			answerErr:    nonInteractive,
			want:         nil,
			wantPrompted: true,
		},
		{
			name:           "no flags, stored true, prompt error propagates",
			stored:         true,
			answerErr:      errors.New("boom"),
			wantErr:        true,
			wantPrompted:   true,
			wantPromptedOn: true,
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
			name:         "stored non-bool garbage: prompts to enable",
			stored:       "yes",
			answer:       true,
			want:         boolPtr(true),
			wantPrompted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompted, promptedOn := false, false
			got, err := resolveProdPack(tt.enableFlag, tt.disableFlag, tt.stored, tt.legacy, confirmStub(tt.answer, tt.answerErr, &prompted, &promptedOn))

			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPrompted, prompted, "prompt invocation")
			assert.Equal(t, tt.wantPromptedOn, promptedOn, "prompt phrasing")

			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}

// A ProdPack decision must reach the wire even when the plan is untouched:
// Upstash applies the add-on change on its own.
func TestResolveProdPackIsIndependentOfPlanChange(t *testing.T) {
	prompted, promptedOn := false, false
	got, err := resolveProdPack(false, false, false, false, confirmStub(true, nil, &prompted, &promptedOn))

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, *got)
}

func TestOptionState(t *testing.T) {
	assert.Equal(t, "unchanged", optionState(map[string]any{}, "eviction"))
	assert.Equal(t, "enabled", optionState(map[string]any{"eviction": true}, "eviction"))
	assert.Equal(t, "disabled", optionState(map[string]any{"eviction": false}, "eviction"))
	assert.Equal(t, "disabled", optionState(map[string]any{"eviction": "yes"}, "eviction"))
}

func TestProdPackState(t *testing.T) {
	assert.Equal(t, "unchanged", prodPackState(nil))
	assert.Equal(t, "enabled", prodPackState(boolPtr(true)))
	assert.Equal(t, "disabled", prodPackState(boolPtr(false)))
}

func TestStripProdPack(t *testing.T) {
	options := map[string]any{"prod_pack": true, "eviction": true}
	stripProdPack(options)
	_, present := options["prod_pack"]
	assert.False(t, present, "prod_pack must be omitted from the legacy options blob")
	assert.Equal(t, true, options["eviction"], "other options untouched")
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
		{
			name:         "explicit enable uses typed input and omits option key",
			enable:       true,
			wantProdPack: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This mirrors production wiring: resolve intent, sanitize the
			// legacy options blob, then pass intent through the typed operation.
			decision, err := resolveProdPack(tt.enable, tt.disable, tt.stored, false, func(bool) (bool, error) {
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

// The plan argument is unrelated to the ProdPack argument: sending the same
// plan back must still carry an explicit ProdPack decision to the provider.
func TestProdPackDecisionSurvivesUnchangedPlan(t *testing.T) {
	client := &captureGenqClient{}

	_, err := gql.UpdateRedisAddOn(context.Background(), client, "addon", "plan", []string{"ord"}, map[string]any{}, map[string]any{}, boolPtr(true))
	require.NoError(t, err)

	assert.Equal(t, 1, client.requests)
	assert.Equal(t, "plan", client.variables["planId"])
	assert.Equal(t, true, client.variables["prodPack"])
}
