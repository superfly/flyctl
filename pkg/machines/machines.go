package machines

import (
	"context"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/flaps"
)

type MachineApp struct {
	FlapsClient *flaps.Client
	VMs         []*api.V1Machine
}

func NewMachineApp(ctx context.Context, app *api.AppCompact) (ma *MachineApp, err error) {
	flapsClient, err := flaps.New(ctx, app)

	ma = &MachineApp{
		FlapsClient: flapsClient,
	}

	return ma, err
}

// func (ma *MachineApp) Lease(strategy string)
// func (ma *MachineApp) Deploy(strategy string)
// func (ma *MachineApp) Release(strategy string)
