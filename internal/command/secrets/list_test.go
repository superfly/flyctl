package secrets

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
)

func TestComputeSecretStatus(t *testing.T) {
	releaseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	secretUpdated := releaseTime.Add(-time.Hour)
	updatedAtStr := secretUpdated.Format(time.RFC3339)

	secret := fly.AppSecret{
		Name:      "MY_SECRET",
		Digest:    "digest",
		UpdatedAt: &updatedAtStr,
	}

	machines := []*fly.Machine{
		{
			ID:    "m1",
			State: "started",
			Config: &fly.MachineConfig{
				Metadata: map[string]string{
					fly.MachineConfigMetadataKeyFlyReleaseVersion: "100",
				},
			},
		},
		{
			ID:    "m2",
			State: "started",
			Config: &fly.MachineConfig{
				Metadata: map[string]string{
					fly.MachineConfigMetadataKeyFlyReleaseVersion: "100",
				},
			},
		},
	}

	releaseTimestamps := map[string]time.Time{
		"100": releaseTime,
	}

	vc := buildVersionCounts(machines, releaseTimestamps)
	assert.Equal(t, StatusDeployed, computeSecretStatus(secret, vc))
}

func TestComputeSecretStatus_NotDeployed(t *testing.T) {
	releaseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	secretUpdated := releaseTime.Add(time.Hour)
	updatedAtStr := secretUpdated.Format(time.RFC3339)

	secret := fly.AppSecret{
		Name:      "MY_SECRET",
		Digest:    "digest",
		UpdatedAt: &updatedAtStr,
	}

	machines := []*fly.Machine{
		{
			ID:    "m1",
			State: "started",
			Config: &fly.MachineConfig{
				Metadata: map[string]string{
					fly.MachineConfigMetadataKeyFlyReleaseVersion: "100",
				},
			},
		},
	}

	releaseTimestamps := map[string]time.Time{
		"100": releaseTime,
	}

	vc := buildVersionCounts(machines, releaseTimestamps)
	assert.Equal(t, StatusStaged, computeSecretStatus(secret, vc))
}

func TestComputeSecretStatus_PartiallyDeployed(t *testing.T) {
	releaseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	secretUpdated := releaseTime.Add(-time.Minute)
	updatedAtStr := secretUpdated.Format(time.RFC3339)

	secret := fly.AppSecret{
		Name:      "MY_SECRET",
		Digest:    "digest",
		UpdatedAt: &updatedAtStr,
	}

	machines := []*fly.Machine{
		{
			ID:    "m1",
			State: "started",
			Config: &fly.MachineConfig{
				Metadata: map[string]string{
					fly.MachineConfigMetadataKeyFlyReleaseVersion: "100",
				},
			},
		},
		{
			ID:    "m2",
			State: "started",
			Config: &fly.MachineConfig{
				Metadata: map[string]string{
					fly.MachineConfigMetadataKeyFlyReleaseVersion: "101",
				},
			},
		},
	}

	releaseTimestamps := map[string]time.Time{
		"100": releaseTime,
		// 101 missing -> treated as not deployed on that machine
	}

	vc := buildVersionCounts(machines, releaseTimestamps)
	assert.Equal(t, StatusPartiallyDeployed, computeSecretStatus(secret, vc))
}

func TestComputeSecretStatus_UnknownScenarios(t *testing.T) {
	now := time.Now()
	nowStr := now.Format(time.RFC3339)
	secret := fly.AppSecret{
		Name:      "MY_SECRET",
		Digest:    "digest",
		UpdatedAt: &nowStr,
	}

	// empty versionCounts (no machines) -> staged
	emptyVC := buildVersionCounts(nil, nil)
	assert.Equal(t, StatusStaged, computeSecretStatus(secret, emptyVC))

	// missing updated_at -> unknown
	machineWithRelease := []*fly.Machine{
		{
			ID:    "m1",
			State: "started",
			Config: &fly.MachineConfig{
				Metadata: map[string]string{
					fly.MachineConfigMetadataKeyFlyReleaseVersion: "100",
				},
			},
		},
	}
	releases := map[string]time.Time{"100": now}
	vcWithRelease := buildVersionCounts(machineWithRelease, releases)
	assert.Equal(t, StatusUnknown, computeSecretStatus(fly.AppSecret{Name: "X", Digest: "d"}, vcWithRelease))
}

func TestFilterRelevantMachines(t *testing.T) {
	machines := []*fly.Machine{
		{ID: "m1", State: "started"},
		{ID: "m2", State: "stopped"},
		{ID: "m3", State: "destroyed"},
		{ID: "m4", State: "destroying"},
		{ID: "m5", State: "suspended"},
	}

	result := filterRelevantMachines(machines)

	assert.Len(t, result, 3)

	states := make([]string, len(result))
	for i, m := range result {
		states[i] = m.State
	}

	assert.Contains(t, states, "started")
	assert.Contains(t, states, "stopped")
	assert.Contains(t, states, "suspended")
	assert.NotContains(t, states, "destroyed")
	assert.NotContains(t, states, "destroying")
}

func TestFilterRelevantMachines_NilInput(t *testing.T) {
	result := filterRelevantMachines(nil)
	assert.Nil(t, result)
}

func TestGetMachineReleaseVersion(t *testing.T) {
	tests := []struct {
		name     string
		machine  *fly.Machine
		expected string
	}{
		{
			name:     "nil machine",
			machine:  nil,
			expected: "",
		},
		{
			name:     "nil config",
			machine:  &fly.Machine{ID: "m1", Config: nil},
			expected: "",
		},
		{
			name: "nil metadata",
			machine: &fly.Machine{
				ID:     "m1",
				Config: &fly.MachineConfig{Metadata: nil},
			},
			expected: "",
		},
		{
			name: "missing release version",
			machine: &fly.Machine{
				ID:     "m1",
				Config: &fly.MachineConfig{Metadata: map[string]string{}},
			},
			expected: "",
		},
		{
			name: "has release version",
			machine: &fly.Machine{
				ID: "m1",
				Config: &fly.MachineConfig{
					Metadata: map[string]string{
						fly.MachineConfigMetadataKeyFlyReleaseVersion: "123",
					},
				},
			},
			expected: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMachineReleaseVersion(tt.machine)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildRowsWithoutStatus_PreservesTimestamps(t *testing.T) {
	created := "2024-01-01T10:00:00Z"
	updated := "2024-01-01T11:00:00Z"
	secrets := []fly.AppSecret{
		{
			Name:      "TEST_SECRET",
			Digest:    "abc123",
			CreatedAt: &created,
			UpdatedAt: &updated,
		},
	}
	rows, secretsWithoutStatus := buildRowsWithoutStatus(secrets)
	assert.Len(t, rows, 1)
	assert.Equal(t, []string{"TEST_SECRET", "abc123"}, rows[0])
	assert.Len(t, secretsWithoutStatus, 1)
	assert.Equal(t, "TEST_SECRET", secretsWithoutStatus[0].Name)
	assert.Equal(t, "abc123", secretsWithoutStatus[0].Digest)
	assert.Equal(t, &created, secretsWithoutStatus[0].CreatedAt)
	assert.Equal(t, &updated, secretsWithoutStatus[0].UpdatedAt)
}

func TestSecretWithStatus_JSONMarshalling(t *testing.T) {
	created := "2024-01-01T10:00:00Z"
	updated := "2024-01-01T11:00:00Z"
	s := SecretWithStatus{
		Name:      "MY_KEY",
		Digest:    "deadbeef",
		CreatedAt: &created,
		UpdatedAt: &updated,
		Status:    StatusDeployed,
	}
	data, err := json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"created_at":"2024-01-01T10:00:00Z"`)
	assert.Contains(t, string(data), `"updated_at":"2024-01-01T11:00:00Z"`)
	assert.Contains(t, string(data), `"status":"Deployed"`)
}
