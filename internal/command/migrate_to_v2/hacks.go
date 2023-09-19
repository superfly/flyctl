package migrate_to_v2

import (
	"context"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
)

// This all *should* work, based on the assumption that the existing app was validated and is running.
// Essentially, we shouldn't make things up here, but extrapolate the *only* possible outcome.

func (m *v2PlatformMigrator) applyHacks(ctx context.Context) {

	m.hackAddProcessToServices(ctx)
	m.hackInferProcessName(ctx)
}

// If there is only one process to the app, we can assume that services are all tied to that one process
// unless they specify another process name
func (m *v2PlatformMigrator) hackAddProcessToServices(ctx context.Context) {
	appProcesses := m.appConfig.ProcessNames()

	if len(appProcesses) != 1 {
		// Apps typically aren't hosting services on multiple process groups, so we can't really
		// make a good prediction here.
		return
	}

	for idx := range m.appConfig.Services {
		if len(m.appConfig.Services[idx].Processes) == 0 {
			m.appConfig.Services[idx].Processes = appProcesses
		}
	}
	if m.appConfig.HTTPService != nil && len(m.appConfig.HTTPService.Processes) == 0 {
		m.appConfig.HTTPService.Processes = appProcesses
	}
}

func (m *v2PlatformMigrator) hackInferProcessName(ctx context.Context) {

	existingNames := m.appConfig.ProcessNames()
	if len(existingNames) > 1 || existingNames[0] != api.MachineProcessGroupApp {
		return
	}

	// There's a single process, and it's unnamed. See if we can infer its name from services
	var listedNames []string
	for _, service := range m.appConfig.AllServices() {
		listedNames = append(listedNames, service.Processes...)
	}
	listedNames = lo.Uniq(listedNames)

	if len(listedNames) == 1 {
		// There's a single name listed, so we can assume that that's the only process.
		m.appConfig.Processes = map[string]string{
			listedNames[0]: "", // an empty cmd will inherit the CMD from the Dockerfile, or from Experimental.Cmd
		}
	}
}
