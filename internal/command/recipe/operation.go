package recipe

import "github.com/superfly/flyctl/api"

func (o *Operation) ProcessSelectors(machines []*api.Machine) []*api.Machine {

	if o.Selector == (Selector{}) {
		return machines
	}

	var targets []*api.Machine

	for _, m := range machines {
		if o.Selector.HealthCheck != (HealthCheckSelector{}) {
			if matchesHealthCheckConstraints(m, o.Selector.HealthCheck) {
				targets = append(targets, m)

			}
		}
	}

	return targets
}

func matchesHealthCheckConstraints(machine *api.Machine, hs HealthCheckSelector) bool {
	for _, check := range machine.Checks {
		if check.Name == hs.Name && check.Output == hs.Value {
			return true
		}
	}

	return false
}
