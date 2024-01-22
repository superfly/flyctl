package flaps

//go:generate go run golang.org/x/tools/cmd/stringer -type=flapsAction

// flapsAction is used to record actions in traces' attributes.
type flapsAction int

const (
	appCreate flapsAction = iota
	machineLaunch
	machineUpdate
	machineStart
	machineWait
	machineStop
	machineRestart
	machineGet
	machineList
	machineDestroy
	machineKill
	machineFindLease
	machineAcquireLease
	machineRefreshLease
	machineReleaseLease
	machineExec
	machinePs
	machineCordon
	machineUncordon
	volumeList
	volumeCreate
	volumetUpdate
	volumeGet
	volumeSnapshotCreate
	volumeSnapshotList
	volumeExtend
	volumeDelete
)
