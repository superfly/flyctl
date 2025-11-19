package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindServeDirectory(t *testing.T) {
	// Create a temporary directory for our test cases
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		packageJson string
		expected    string
	}{
		{
			name: "serve dist",
			packageJson: `{
				"scripts": {
					"serve": "serve dist"
				}
			}`,
			expected: "dist",
		},
		{
			name: "npx serve build",
			packageJson: `{
				"scripts": {
					"serve": "npx serve build"
				}
			}`,
			expected: "build",
		},
		{
			name: "serve with flags",
			packageJson: `{
				"scripts": {
					"serve": "serve -s public"
				}
			}`,
			expected: "public",
		},
		{
			name: "vite preview",
			packageJson: `{
				"scripts": {
					"serve": "vite preview"
				}
			}`,
			expected: "dist", // vite defaults to dist
		},
		{
			name: "no serve script",
			packageJson: `{
				"scripts": {
					"test": "echo test"
				}
			}`,
			expected: "",
		},
		{
			name: "build script only",
			packageJson: `{
				"scripts": {
					"build": "npm run build"
				}
			}`,
			expected: "",
		},
		{
			name: "precedence serve > dev > web",
			packageJson: `{
				"scripts": {
					"dev": "serve public",
					"web": "serve build",
					"serve": "serve dist"
				}
			}`,
			expected: "dist",
		},
		{
			name: "precedence dev > web",
			packageJson: `{
				"scripts": {
					"dev": "serve public",
					"web": "serve build"
				}
			}`,
			expected: "public",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a sub-directory for each test case to avoid conflicts
			testDir := filepath.Join(tmpDir, tt.name)
			err := os.MkdirAll(testDir, 0755)
			require.NoError(t, err)

			// Write package.json
			err = os.WriteFile(filepath.Join(testDir, "package.json"), []byte(tt.packageJson), 0644)
			require.NoError(t, err)

			// Run the function
			result := findServeDirectory(testDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractDirectory(t *testing.T) {
	tests := []struct {
		script   string
		expected string
	}{
		{"serve dist", "dist"},
		{"npx serve build", "build"},
		{"serve -s public", "public"},
		{"npx serve -s public", "public"},
		{"vite preview", "dist"},
		{"npm run build", ""},
		{"echo 'building'", ""},
	}

	for _, tt := range tests {
		t.Run(tt.script, func(t *testing.T) {
			result := extractDirectory(tt.script)
			assert.Equal(t, tt.expected, result)
		})
	}
}
