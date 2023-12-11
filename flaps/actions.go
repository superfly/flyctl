package flaps

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
	volumeSnapshot
	volumeExtend
	volumeDelete
)

func (a *flapsAction) String() string {
	switch *a {
	case appCreate:
		return "app_create"
	case machineLaunch:
		return "machine_launch"
	case machineUpdate:
		return "machine_update"
	case machineStart:
		return "machine_start"
	case machineWait:
		return "machine_wait"
	case machineStop:
		return "machine_stop"
	case machineRestart:
		return "machine_restart"
	case machineGet:
		return "machine_get"
	case machineList:
		return "machine_list"
	case machineDestroy:
		return "machine_destroy"
	case machineKill:
		return "machine_kill"
	case machineFindLease:
		return "machine_find_lease"
	case machineAcquireLease:
		return "machine_acquire_lease"
	case machineRefreshLease:
		return "machine_refresh_lease"
	case machineReleaseLease:
		return "machine_release_lease"
	case machineExec:
		return "machine_exec"
	case machinePs:
		return "machine_ps"
	case machineCordon:
		return "machine_cordon"
	case machineUncordon:
		return "machine_cordon"
	case volumeCreate:
		return "volume_create"
	case volumeGet:
		return "volume_get"
	case volumeList:
		return "volume_list"
	case volumetUpdate:
		return "volume_update"
	case volumeSnapshot:
		return "volume_snapshot"
	case volumeExtend:
		return "volume_extend"
	case volumeDelete:
		return "volume_delete"
	default:
		return "machine_unknown_action"
	}
}
