package mpg

// PlanDetails holds the details for each managed postgres plan.
type PlanDetails struct {
	Name       string
	CPU        string
	Memory     string
	PricePerMo int
	// Hidden plans are taken by --plan but left out of the plan prompt and
	// the lists of plans, until they launch.
	Hidden bool
}

var MPGPlans = map[string]PlanDetails{
	// The API offers dev-256 only to organizations with its feature flag,
	// and turns it down for the others.
	"dev-256": {
		Name:       "Dev 256 MB",
		CPU:        "Shared x 1",
		Memory:     "256 MB",
		PricePerMo: 5,
		Hidden:     true,
	},
	"basic": {
		Name:       "Basic",
		CPU:        "Shared x 2",
		Memory:     "1 GB",
		PricePerMo: 38,
	},
	"starter": {
		Name:       "Starter",
		CPU:        "Shared x 2",
		Memory:     "2 GB",
		PricePerMo: 72,
	},
	"launch": {
		Name:       "Launch",
		CPU:        "Performance x 2",
		Memory:     "8 GB",
		PricePerMo: 282,
	},
	"scale": {
		Name:       "Scale",
		CPU:        "Performance x 4",
		Memory:     "33 GB",
		PricePerMo: 962,
	},
	"Performance": {
		Name:       "Performance",
		CPU:        "Performance x 8",
		Memory:     "64 GB",
		PricePerMo: 1922,
	},
}
