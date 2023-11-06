package api

import "time"

type Volume struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	State              string    `json:"state"`
	SizeGb             int       `json:"size_gb"`
	Region             string    `json:"region"`
	Zone               string    `json:"zone"`
	Encrypted          bool      `json:"encrypted"`
	AttachedMachine    *string   `json:"attached_machine_id"`
	AttachedAllocation *string   `json:"attached_alloc_id"`
	CreatedAt          time.Time `json:"created_at"`
	HostDedicationID   string    `json:"host_dedication_id"`
	SnapshotRetention  int       `json:"snapshot_retention"`
}

func (v Volume) IsAttached() bool {
	return v.AttachedMachine != nil || v.AttachedAllocation != nil
}

type CreateVolumeRequest struct {
	Name              string `json:"name"`
	Region            string `json:"region"`
	SizeGb            *int   `json:"size_gb"`
	Encrypted         *bool  `json:"encrypted"`
	RequireUniqueZone *bool  `json:"require_unique_zone"`
	MachinesOnly      *bool  `json:"machines_only"`
	SnapshotRetention *int   `json:"snapshot_retention"`

	// restore from snapshot
	SnapshotID *string `json:"snapshot_id"`
	// fork from remote volume
	SourceVolumeID *string `json:"source_volume_id"`

	ComputeRequirements *MachineGuest `json:"compute"`
}

type UpdateVolumeRequest struct {
	SnapshotRetention *int `json:"snapshot_retention"`
}

type VolumeSnapshot struct {
	ID        string    `json:"id"`
	Size      int       `json:"size"`
	Digest    string    `json:"digest"`
	CreatedAt time.Time `json:"created_at"`
}
