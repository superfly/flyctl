package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/superfly/flyctl/helpers"
)

func configureStatic(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// Check for index.html at root or in a subdirectory pointed to by a serve script
	contentRoot := sourceDir
	if !helpers.FileExists(filepath.Join(sourceDir, "index.html")) {
		// Try to find index.html in a subdirectory specified by a serve/dev script in package.json
		if serveDir := findServeDirectory(sourceDir); serveDir != "" {
			contentRoot = filepath.Join(sourceDir, serveDir)
			if !helpers.FileExists(filepath.Join(contentRoot, "index.html")) {
				return nil, nil
			}
		} else {
			return nil, nil
		}
	}

	s := &SourceInfo{
		Family: "Static",
		Port:   8080,
	}

	// If content is in a subdirectory, we need to adjust what we copy
	if contentRoot != sourceDir {
		// Extract the relative path from sourceDir
		relPath, _ := filepath.Rel(sourceDir, contentRoot)
		// Set Version to show the base directory in detection message
		s.Version = "(base path: " + relPath + ")"
		// Generate template with the specific subdirectory
		vars := map[string]interface{}{"contentDir": relPath}
		s.Files = templatesExecute("templates/static", vars)
	} else {
		s.Files = templates("templates/static")
	}

	return s, nil
}

// findServeDirectory looks for a serve/dev script in package.json and extracts the served directory
func findServeDirectory(sourceDir string) string {
	data, err := os.ReadFile(filepath.Join(sourceDir, "package.json"))
	if err != nil {
		return ""
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}

	scripts, ok := pkg["scripts"].(map[string]interface{})
	if !ok {
		return ""
	}

	// Check for common serve script names
	for _, scriptName := range []string{"serve", "dev", "web"} {
		if script, exists := scripts[scriptName]; exists {
			if scriptStr, ok := script.(string); ok {
				// Try to extract directory from commands like "npx serve demo" or "vite --host"
				if dir := extractDirectory(scriptStr); dir != "" {
					return dir
				}
			}
		}
	}

	return ""
}

// extractDirectory tries to extract a directory path from common serve commands
func extractDirectory(script string) string {
	// Match patterns like "npx serve dist", "serve -s dist", "serve -s public"
	// This regex looks for "serve" followed optionally by flags (starting with -), and then captures the directory argument
	re := regexp.MustCompile(`(?:npx\s+)?serve\s+(?:-[a-zA-Z0-9-]+\s+)*(\S+)`)
	if matches := re.FindStringSubmatch(script); len(matches) > 1 {
		// Filter out flags if they were accidentally captured (though the regex should handle it)
		if !strings.HasPrefix(matches[1], "-") {
			return matches[1]
		}
	}

	// Handle vite
	if strings.Contains(script, "vite") {
		return "dist"
	}

	// For fallback directory detection, only check scripts that don't contain
	// commands that might accidentally match (like echo, npm run, yarn, etc.)
	if strings.Contains(script, "npm run") || strings.Contains(script, "yarn") || 
		strings.Contains(script, "echo") || strings.Contains(script, "node ") {
		return ""
	}

	// Handle explicit references to common build directories in other scripts
	if strings.Contains(script, "dist") {
		return "dist"
	}

	// Check for other common output directories
	for _, dir := range []string{"build", "public", "out"} {
		if strings.Contains(script, dir) {
			return dir
		}
	}

	return ""
}
