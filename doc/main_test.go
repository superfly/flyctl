package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneratedCommandDocumentationUsesCanonicalURLs(t *testing.T) {
	t.Chdir(t.TempDir())
	main()

	files, err := filepath.Glob("out/*.mdx")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	links := regexp.MustCompile(`\]\(/flyctl/cmd/([^\s)#]+)\)`)
	seen := map[string]bool{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		text := string(data)
		name := strings.TrimSuffix(filepath.Base(file), ".mdx")
		seen[name] = true
		require.Contains(t, text, "\nsource_url: https://docs.fly.io/flyctl/cmd/"+name+"\n", file)
		require.NotContains(t, text, "https://fly.io/docs/", file)
		for _, link := range links.FindAllStringSubmatch(text, -1) {
			_, err := os.Stat(filepath.Join("out", link[1]+".mdx"))
			require.NoError(t, err, "broken command link in %s: %s", file, link[0])
		}
	}
	for _, name := range []string{"fly_help", "fly_apps", "fly_auth_login", "fly_volumes_create"} {
		require.True(t, seen[name], "expected generated page %s", name)
	}
}
