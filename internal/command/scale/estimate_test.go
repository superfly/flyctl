package scale

import (
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
)

func TestScaleCountEstimateChanges(t *testing.T) {
	sizeGB := 10
	changes := scaleCountEstimateChanges([]*planItem{
		{
			GroupName: "app",
			Region:    "iad",
			Delta:     2,
			LaunchMachineInput: &fly.LaunchMachineInput{
				Region: "iad",
				Config: &fly.MachineConfig{Guest: &fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 256}},
			},
			CreateVolumeRequest: &fly.CreateVolumeRequest{Region: "iad", SizeGb: &sizeGB},
		},
		{
			GroupName: "worker",
			Region:    "ord",
			Delta:     -1,
			Machines: []*fly.Machine{
				{ID: "machine-1", Region: "ord", Config: &fly.MachineConfig{Guest: &fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 512}}},
			},
		},
	})

	require.Len(t, changes, 3)
	require.Equal(t, "machine", changes[0].Kind)
	require.Equal(t, "create", changes[0].Action)
	require.Equal(t, "app:iad", changes[0].Ref)
	require.Equal(t, 2, changes[0].Count)
	require.IsType(t, &fly.LaunchMachineInput{}, changes[0].Desired)

	require.Equal(t, "volume", changes[1].Kind)
	require.Equal(t, "create", changes[1].Action)
	require.Equal(t, "app:iad:volume", changes[1].Ref)
	require.Equal(t, 2, changes[1].Count)
	require.Equal(t, scaleVolumeSpec{Region: "iad", SizeGB: 10}, changes[1].Desired)

	require.Equal(t, "machine", changes[2].Kind)
	require.Equal(t, "destroy", changes[2].Action)
	require.Equal(t, "machine-1", changes[2].Ref)
	require.Equal(t, 1, changes[2].Count)
	require.IsType(t, &fly.Machine{}, changes[2].Current)
}
