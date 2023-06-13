package migrate_to_v2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/jpillora/backoff"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"
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
	m.appLock = ""
	return nil
}

func (m *v2PlatformMigrator) scaleNomadToZero(ctx context.Context) error {
	err := scaleNomadToZero(ctx, m.appCompact, m.appLock, lo.Keys(m.rawNomadScaleMapping))
	if err != nil {
		return err
	}
	m.recovery.scaledToZero = true
	return nil
}

func scaleNomadToZero(ctx context.Context, app *api.AppCompact, lock string, vmGroups []string) error {

	gqlClient := client.FromContext(ctx).API().GenqClient

	input := gql.SetVMCountInput{
		AppId:  app.Name,
		LockId: lock,
		GroupCounts: lo.Map(vmGroups, func(name string, _ int) gql.VMCountInput {
			return gql.VMCountInput{Group: name, Count: 0}
		}),
	}

	if len(input.GroupCounts) > 0 {
		_, err := gql.SetNomadVMCount(ctx, gqlClient, input)
		if err != nil {
			return err
		}
	}
	err := waitForAllocsZero(ctx, app)
	if err != nil {
		return err
	}

	return nil
}

func waitForAllocsZero(ctx context.Context, app *api.AppCompact) error {
	io := iostreams.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	s := spinner.New(spinner.CharSets[9], 200*time.Millisecond)
	s.Writer = io.ErrOut
	s.Prefix = fmt.Sprintf("Waiting for nomad allocs for '%s' to be destroyed ", app.Name)
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
			currentAllocs, err := apiClient.GetAllocations(ctx, app.Name, false)
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
