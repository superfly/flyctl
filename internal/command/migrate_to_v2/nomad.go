package migrate_to_v2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/jpillora/backoff"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/gql"
)

func (m *v2PlatformMigrator) lockApp(ctx context.Context) error {
	_ = `# @genqlient
	mutation LockApp($input:LockAppInput!) {
        lockApp(input:$input) {
			lockId
			expiration
        }
	}
	`
	input := gql.LockAppInput{
		AppId: m.appConfig.AppName,
	}
	resp, err := gql.LockApp(ctx, m.gqlClient, input)
	if err != nil {
		return err
	}

	m.appLock = resp.LockApp.LockId
	m.recovery.appLocked = true
	return nil
}

func (m *v2PlatformMigrator) unlockApp(ctx context.Context) error {
	_ = `# @genqlient
	mutation UnlockApp($input:UnlockAppInput!) {
		unlockApp(input:$input) {
			app { id }
		}
	}
	`
	input := gql.UnlockAppInput{
		AppId:  m.appConfig.AppName,
		LockId: m.appLock,
	}
	_, err := gql.UnlockApp(ctx, m.gqlClient, input)
	if err != nil {
		return err
	}
	m.recovery.appLocked = false
	return nil
}

func (m *v2PlatformMigrator) scaleNomadToZero(ctx context.Context) error {
	input := gql.SetVMCountInput{
		AppId:  m.appConfig.AppName,
		LockId: m.appLock,
		GroupCounts: lo.MapToSlice(m.oldVmCounts, func(name string, count int) gql.VMCountInput {
			return gql.VMCountInput{Group: name, Count: 0}
		}),
	}

	if len(input.GroupCounts) > 0 {

		_, err := gql.SetNomadVMCount(ctx, m.gqlClient, input)
		if err != nil {
			return err
		}
	}
	err := m.waitForAllocsZero(ctx)
	if err != nil {
		return err
	}

	m.recovery.scaledToZero = true
	return nil
}

func (m *v2PlatformMigrator) waitForAllocsZero(ctx context.Context) error {
	s := spinner.New(spinner.CharSets[9], 200*time.Millisecond)
	s.Writer = m.io.ErrOut
	s.Prefix = fmt.Sprintf("Waiting for nomad allocs for '%s' to be destroyed ", m.appCompact.Name)
	s.Start()
	defer s.Stop()

	timeout := time.After(1 * time.Hour)
	b := &backoff.Backoff{
		Min:    2 * time.Second,
		Max:    5 * time.Minute,
		Factor: 1.2,
		Jitter: true,
	}
	for {
		select {
		case <-time.After(b.Duration()):
			currentAllocs, err := m.apiClient.GetAllocations(ctx, m.appCompact.Name, false)
			if err != nil {
				return err
			}
			if len(currentAllocs) == 0 {
				return nil
			}
		case <-timeout:
			return errors.New("nomad allocs never reached zero, timed out")
		}
	}
}
