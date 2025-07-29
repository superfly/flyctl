package plan

import "context"

type contextKey string

const PlanStepKey contextKey = "planStep"

func GetPlanStep(ctx context.Context) string {
	step := ctx.Value(PlanStepKey)
	if step == nil {
		return ""
	} else {
		return step.(string)
	}
}
