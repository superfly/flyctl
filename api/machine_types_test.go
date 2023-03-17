package api

import (
	"testing"
)

func TestIsReleaseCommandMachine(t *testing.T) {
	type testcase struct {
		name     string
		machine  Machine
		expected bool
	}

	cases := []testcase{
		{
			name:     "release machine using 'process_group'",
			expected: true,
			machine: Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"process_group": "release_command",
					},
				},
			},
		},
		{
			name:     "release machine using 'fly_process_group'",
			expected: true,
			machine: Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"fly_process_group": "fly_app_release_command",
					},
				},
			},
		},
		{
			name:     "non-release machine using 'fly_process_group'",
			expected: false,
			machine: Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"fly_process_group": "web",
					},
				},
			},
		},
		{
			name:     "non-release machine using 'process_group'",
			expected: false,
			machine: Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"process_group": "web",
					},
				},
			},
		},
	}

	for _, tc := range cases {
		result := tc.machine.IsReleaseCommandMachine()
		if result != tc.expected {
			t.Errorf("%s, got '%v', want '%v'", tc.name, result, tc.expected)
		}
	}
}

func TestGetProcessGroup(t *testing.T) {
	type testcase struct {
		name     string
		machine  *Machine
		expected string
	}

	cases := []testcase{
		{
			name:     "machine with only 'process_group'",
			expected: "web",
			machine: &Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"process_group": "web",
					},
				},
			},
		},
		{
			name:     "machine with both 'process_group' & 'fly_process_group'",
			expected: "app",
			machine: &Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"process_group":     "web",
						"fly_process_group": "app",
					},
				},
			},
		},
		{
			name:     "machine with only 'fly_process_group'",
			expected: "web",
			machine: &Machine{
				Config: &MachineConfig{
					Metadata: map[string]string{
						"fly_process_group": "web",
					},
				},
			},
		},
	}

	for _, tc := range cases {
		result := tc.machine.ProcessGroup()
		if result != tc.expected {
			t.Errorf("%s, got '%v', want '%v'", tc.name, result, tc.expected)
		}
	}
}

func TestMachineGuest_SetSize(t *testing.T) {
	var guest MachineGuest

	if err := guest.SetSize("unknown"); err == nil {
		t.Error("want error for invalid kind")
	}

	if err := guest.SetSize("shared-cpu-3x"); err == nil {
		t.Error("want error for invalid preset name")
	}

	if err := guest.SetSize("performance-4x"); err != nil {
		t.Errorf("got error for valid preset name: %v", err)
	} else {
		if guest.CPUs != 4 {
			t.Errorf("Expected 4 cpus, got: %v", guest.CPUs)
		}
		if guest.CPUKind != "performance" {
			t.Errorf("Expected performance cpu kind, got: %v", guest.CPUKind)
		}
		if guest.MemoryMB != 8192 {
			t.Errorf("Expected 8192 MB of memory , got: %v", guest.MemoryMB)
		}
	}
}
