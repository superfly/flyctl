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

// Runtime messages never reach the generated pages, so scan the source too:
// no links to the retired fly.io/docs host, and no docs URL followed by a
// sentence-ending period, which terminals can fold into the link (a 404).
func TestSourceDocumentationLinks(t *testing.T) {
	legacy := regexp.MustCompile(`https?://fly\.io/docs\b`)
	trailingDot := regexp.MustCompile(`https://docs\.fly\.io/[^\s"'` + "`" + `)]*\.(\s|"|'|` + "`" + `|\\n|$)`)
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "out":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".yml", ".yaml", ".toml", ".tmpl":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if legacy.MatchString(line) {
				t.Errorf("%s:%d: link to https://fly.io/docs; use https://docs.fly.io", path, i+1)
			}
			if trailingDot.MatchString(line) {
				t.Errorf("%s:%d: docs URL followed by a period", path, i+1)
			}
		}
		return nil
	})
	require.NoError(t, err)
}
