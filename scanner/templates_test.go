package scanner

import (
	"bytes"
	"io/fs"
	"path"
	"testing"
	"text/template"

	"github.com/moby/buildkit/frontend/dockerfile/dockerignore"
	"github.com/moby/patternmatcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every template that ships a .dockerignore keeps the repository's .git
// directory out of the build context, so it never ends up in the image.
// Templates without one get launch's default .dockerignore, which also
// leaves out .git.
func TestTemplatesExcludeGitFromBuildContext(t *testing.T) {
	entries, err := fs.ReadDir(content, "templates")
	require.NoError(t, err)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ignorePath := path.Join("templates", entry.Name(), ".dockerignore")
		if _, err := fs.Stat(content, ignorePath); err != nil {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			raw, err := fs.ReadFile(content, ignorePath)
			require.NoError(t, err)

			// Some .dockerignore templates have conditional sections.
			tmpl, err := template.New(".dockerignore").Parse(string(raw))
			require.NoError(t, err)
			var rendered bytes.Buffer
			require.NoError(t, tmpl.Execute(&rendered, map[string]any{}))

			excludes, err := dockerignore.ReadAll(&rendered)
			require.NoError(t, err)
			pm, err := patternmatcher.New(excludes)
			require.NoError(t, err)

			for _, file := range []string{".git", ".git/config", ".git/HEAD"} {
				excluded, err := pm.MatchesOrParentMatches(file)
				require.NoError(t, err)
				assert.True(t, excluded, "%s should be excluded from the build context", file)
			}
		})
	}
}
