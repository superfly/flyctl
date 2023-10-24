package flaps

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
)

var destroyedVolumeStates = []string{"scheduling_destroy", "fork_cleanup", "waiting_for_detach", "pending_destroy", "destroying"}

func (f *Client) sendRequestVolumes(ctx context.Context, method, endpoint string, in, out interface{}, headers map[string][]string) error {
	endpoint = fmt.Sprintf("/apps/%s/volumes%s", f.appName, endpoint)
	return f._sendRequest(ctx, method, endpoint, in, out, headers)
}

func (f *Client) GetAllVolumes(ctx context.Context) ([]api.Volume, error) {
	listVolumesEndpoint := ""

	out := make([]api.Volume, 0)

	err := f.sendRequestVolumes(ctx, http.MethodGet, listVolumesEndpoint, nil, &out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}
	return out, nil
}

func (f *Client) GetVolumes(ctx context.Context) ([]api.Volume, error) {
	volumes, err := f.GetAllVolumes(ctx)
	if err != nil {
		return nil, err
	}
	return lo.Filter(volumes, func(v api.Volume, _ int) bool {
		return !slices.Contains(destroyedVolumeStates, v.State)
	}), nil
}

func (f *Client) CreateVolume(ctx context.Context, req api.CreateVolumeRequest) (*api.Volume, error) {
	createVolumeEndpoint := ""

	out := new(api.Volume)

	err := f.sendRequestVolumes(ctx, http.MethodPost, createVolumeEndpoint, req, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create volume: %w", err)
	}
	return out, nil
}

func (f *Client) UpdateVolume(ctx context.Context, volumeId string, req api.UpdateVolumeRequest) (*api.Volume, error) {
	updateVolumeEndpoint := fmt.Sprintf("/%s", volumeId)

	out := new(api.Volume)

	err := f.sendRequestVolumes(ctx, http.MethodPut, updateVolumeEndpoint, req, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to update volume: %w", err)
	}
	return out, nil
}

func (f *Client) GetVolume(ctx context.Context, volumeId string) (*api.Volume, error) {
	getVolumeEndpoint := fmt.Sprintf("/%s", volumeId)

	out := new(api.Volume)

	err := f.sendRequestVolumes(ctx, http.MethodGet, getVolumeEndpoint, nil, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume %s: %w", volumeId, err)
	}
	return out, nil
}

func (f *Client) GetVolumeSnapshots(ctx context.Context, volumeId string) ([]api.VolumeSnapshot, error) {
	getVolumeSnapshotsEndpoint := fmt.Sprintf("/%s/snapshots", volumeId)

	out := make([]api.VolumeSnapshot, 0)

	err := f.sendRequestVolumes(ctx, http.MethodGet, getVolumeSnapshotsEndpoint, nil, &out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume %s snapshots: %w", volumeId, err)
	}
	return out, nil
}

type ExtendVolumeRequest struct {
	SizeGB int `json:"size_gb"`
}

type ExtendVolumeResponse struct {
	Volume       *api.Volume `json:"volume"`
	NeedsRestart bool        `json:"needs_restart"`
}

func (f *Client) ExtendVolume(ctx context.Context, volumeId string, size_gb int) (*api.Volume, bool, error) {
	extendVolumeEndpoint := fmt.Sprintf("/%s/extend", volumeId)

	req := ExtendVolumeRequest{
		SizeGB: size_gb,
	}

	out := new(ExtendVolumeResponse)

	err := f.sendRequestVolumes(ctx, http.MethodPut, extendVolumeEndpoint, req, out, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to extend volume %s: %w", volumeId, err)
	}
	return out.Volume, out.NeedsRestart, nil
}

func (f *Client) DeleteVolume(ctx context.Context, volumeId string) (*api.Volume, error) {
	destroyVolumeEndpoint := fmt.Sprintf("/%s", volumeId)

	out := new(api.Volume)

	err := f.sendRequestVolumes(ctx, http.MethodDelete, destroyVolumeEndpoint, nil, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to destroy volume %s: %w", volumeId, err)
	}
	return out, nil
}
