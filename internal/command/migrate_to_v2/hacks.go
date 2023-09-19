package migrate_to_v2

import "context"

// This all *should* work, based on the assumption that the existing app was validated and is running.
// Essentially, we shouldn't make things up here, but extrapolate the *only* possible outcome.

func (m *v2PlatformMigrator) applyHacks(ctx context.Context) {

	m.hackAddProcessToServices(ctx)
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
