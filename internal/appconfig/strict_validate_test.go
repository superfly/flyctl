package appconfig

import (
	"testing"
)

func TestStrictValidate(t *testing.T) {
	tests := []struct {
		name                   string
		config                 map[string]any
		wantUnrecognizedSections []string
		wantUnrecognizedKeys   map[string][]string
		wantErr                bool
	}{
		{
			name: "valid config",
			config: map[string]any{
				"app":            "test-app",
				"primary_region": "iad",
				"build": map[string]any{
					"builder": "dockerfile",
				},
				"env": map[string]any{
					"NODE_ENV": "production",
				},
			},
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     nil,
			wantErr:                  false,
		},
		{
			name: "unrecognized top-level section",
			config: map[string]any{
				"app":             "test-app",
				"unknown_section": map[string]any{"key": "value"},
			},
			wantUnrecognizedSections: []string{"unknown_section"},
			wantUnrecognizedKeys:     nil,
			wantErr:                  false,
		},
		{
			name: "unrecognized key in build section",
			config: map[string]any{
				"app": "test-app",
				"build": map[string]any{
					"builder":     "dockerfile",
					"unknown_key": "value",
				},
			},
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     map[string][]string{"build": {"unknown_key"}},
			wantErr:                  false,
		},
		{
			name: "unrecognized key in services array",
			config: map[string]any{
				"app": "test-app",
				"services": []any{
					map[string]any{
						"internal_port": 8080,
						"protocol":      "tcp",
						"invalid_key":   "value",
					},
				},
			},
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     map[string][]string{"services[0]": {"invalid_key"}},
			wantErr:                  false,
		},
		{
			name: "unrecognized key in nested ports",
			config: map[string]any{
				"app": "test-app",
				"services": []any{
					map[string]any{
						"internal_port": 8080,
						"ports": []any{
							map[string]any{
								"port":        80,
								"invalid_key": "value",
							},
						},
					},
				},
			},
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     map[string][]string{"services[0].ports[0]": {"invalid_key"}},
			wantErr:                  false,
		},
		{
			name: "env and processes sections allow any keys",
			config: map[string]any{
				"app": "test-app",
				"env": map[string]any{
					"ANY_KEY":      "value1",
					"ANOTHER_KEY":  "value2",
				},
				"processes": map[string]any{
					"web":    "npm start",
					"worker": "npm run worker",
				},
			},
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     nil,
			wantErr:                  false,
		},
		{
			name: "checks section with arbitrary keys",
			config: map[string]any{
				"app": "test-app",
				"checks": map[string]any{
					"health_check": map[string]any{
						"type":     "http",
						"port":     8080,
						"path":     "/health",
						"interval": "10s",
					},
				},
			},
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     nil,
			wantErr:                  false,
		},
		{
			name: "unrecognized key in checks value",
			config: map[string]any{
				"app": "test-app",
				"checks": map[string]any{
					"health_check": map[string]any{
						"type":        "http",
						"invalid_key": "value",
					},
				},
			},
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     map[string][]string{"checks.health_check": {"invalid_key"}},
			wantErr:                  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := StrictValidate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("StrictValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if err != nil {
				return
			}

			// Check unrecognized sections
			if len(result.UnrecognizedSections) != len(tt.wantUnrecognizedSections) {
				t.Errorf("StrictValidate() UnrecognizedSections = %v, want %v", result.UnrecognizedSections, tt.wantUnrecognizedSections)
			} else {
				for i, section := range result.UnrecognizedSections {
					if section != tt.wantUnrecognizedSections[i] {
						t.Errorf("StrictValidate() UnrecognizedSections[%d] = %v, want %v", i, section, tt.wantUnrecognizedSections[i])
					}
				}
			}

			// Check unrecognized keys
			if len(result.UnrecognizedKeys) != len(tt.wantUnrecognizedKeys) {
				t.Errorf("StrictValidate() UnrecognizedKeys = %v, want %v", result.UnrecognizedKeys, tt.wantUnrecognizedKeys)
			} else {
				for section, keys := range tt.wantUnrecognizedKeys {
					gotKeys, ok := result.UnrecognizedKeys[section]
					if !ok {
						t.Errorf("StrictValidate() missing UnrecognizedKeys for section %s", section)
						continue
					}
					if len(gotKeys) != len(keys) {
						t.Errorf("StrictValidate() UnrecognizedKeys[%s] = %v, want %v", section, gotKeys, keys)
					} else {
						for i, key := range keys {
							if gotKeys[i] != key {
								t.Errorf("StrictValidate() UnrecognizedKeys[%s][%d] = %v, want %v", section, i, gotKeys[i], key)
							}
						}
					}
				}
			}
		})
	}
}

func TestFormatStrictValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		result   *StrictValidateResult
		expected string
	}{
		{
			name: "no errors",
			result: &StrictValidateResult{
				UnrecognizedSections: []string{},
				UnrecognizedKeys:     map[string][]string{},
			},
			expected: "",
		},
		{
			name: "only unrecognized sections",
			result: &StrictValidateResult{
				UnrecognizedSections: []string{"section1", "section2"},
				UnrecognizedKeys:     map[string][]string{},
			},
			expected: "Unrecognized sections:\n  - section1\n  - section2",
		},
		{
			name: "only unrecognized keys",
			result: &StrictValidateResult{
				UnrecognizedSections: []string{},
				UnrecognizedKeys: map[string][]string{
					"build":       {"key1", "key2"},
					"services[0]": {"key3"},
				},
			},
			expected: "Unrecognized keys:\n  - build: key1\n  - build: key2\n  - services[0]: key3",
		},
		{
			name: "both sections and keys",
			result: &StrictValidateResult{
				UnrecognizedSections: []string{"unknown_section"},
				UnrecognizedKeys: map[string][]string{
					"build": {"invalid_key"},
				},
			},
			expected: "Unrecognized sections:\n  - unknown_section\n\nUnrecognized keys:\n  - build: invalid_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatStrictValidationErrors(tt.result)
			if got != tt.expected {
				t.Errorf("FormatStrictValidationErrors() = %q, want %q", got, tt.expected)
			}
		})
	}
}