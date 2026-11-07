package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scanFiles writes files into a fresh directory and scans it as fly launch
// does, from inside that directory.
func scanFiles(t *testing.T, files map[string]string) *SourceInfo {
	t.Helper()
	t.Setenv("OPT_OUT_GITHUB_ACTIONS", "1")

	dir := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	}
	t.Chdir(dir)

	si, err := Scan(dir, &ScannerConfig{Mode: "launch", SkipHealthcheck: true})
	require.NoError(t, err)

	return si
}

// Scan picks the first scanner that recognizes the source, so the order of
// the scanners decides what kind of app a directory is. These cases pin down
// the families and the order. Scanners that run a toolchain while scanning
// (Rails, Phoenix, Laravel, JavaScript) have their own tests.
func TestScanDetectsFamily(t *testing.T) {
	const (
		goMod      = "module example.com/app\n\ngo 1.22\n"
		dockerfile = "FROM alpine\nEXPOSE 3000\n"
		django     = "Django==5.0\n"
	)
	cargo := func(dep string) string {
		return "[package]\nname = \"hello\"\nversion = \"0.1.0\"\n\n[dependencies]\n" + dep + " = \"1\"\n"
	}

	for _, tc := range []struct {
		name       string
		files      map[string]string
		wantFamily string
		wantPort   int
	}{
		{name: "nothing", files: map[string]string{"README.md": "hi\n"}},
		{name: "Dockerfile", files: map[string]string{"Dockerfile": dockerfile}, wantFamily: "Dockerfile", wantPort: 3000},
		{name: "Containerfile", files: map[string]string{"Containerfile": dockerfile}, wantFamily: "Dockerfile", wantPort: 3000},
		{name: "Go", files: map[string]string{"go.mod": goMod, "go.sum": ""}, wantFamily: "Go", wantPort: 8080},
		{name: "Rust", files: map[string]string{"Cargo.toml": cargo("serde")}, wantFamily: "Rust", wantPort: 8080},
		{name: "Rocket", files: map[string]string{"Cargo.toml": cargo("rocket")}, wantFamily: "Rocket", wantPort: 8080},
		{name: "Actix Web", files: map[string]string{"Cargo.toml": cargo("actix-web")}, wantFamily: "Actix Web", wantPort: 8080},
		{name: "Axum", files: map[string]string{"Cargo.toml": cargo("axum")}, wantFamily: "Axum", wantPort: 8080},
		{name: "Deno", files: map[string]string{"deno.json": "{}\n"}, wantFamily: "Deno", wantPort: 8080},
		{name: "static", files: map[string]string{"index.html": "<h1>hi</h1>\n"}, wantFamily: "Static", wantPort: 8080},
		{name: "Django", files: map[string]string{"requirements.txt": django}, wantFamily: "Django", wantPort: 8000},
		{name: "Bridgetown", files: map[string]string{"Gemfile": "gem 'bridgetown'\n"}, wantFamily: "Bridgetown"},
		{name: "Lucky", files: map[string]string{"shard.yml": "dependencies:\n  lucky:\n"}, wantFamily: "Lucky"},

		// A framework scanner beats a Dockerfile: the framework knows how
		// to deploy and may bring a better Dockerfile.
		{
			name:       "Django with a Dockerfile",
			files:      map[string]string{"requirements.txt": django, "Dockerfile": dockerfile},
			wantFamily: "Django", wantPort: 8000,
		},
		// A Dockerfile beats a language scanner: it says how to build.
		{
			name:       "Go with a Dockerfile",
			files:      map[string]string{"go.mod": goMod, "go.sum": "", "Dockerfile": dockerfile},
			wantFamily: "Dockerfile", wantPort: 3000,
		},
		{
			name:       "static site with a Dockerfile",
			files:      map[string]string{"index.html": "<h1>hi</h1>\n", "Dockerfile": dockerfile},
			wantFamily: "Dockerfile", wantPort: 3000,
		},
		// An index.html in a Go repo (docs, templates) doesn't make it static.
		{
			name:       "Go with an index.html",
			files:      map[string]string{"go.mod": goMod, "go.sum": "", "index.html": "<h1>hi</h1>\n"},
			wantFamily: "Go", wantPort: 8080,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			si := scanFiles(t, tc.files)

			if tc.wantFamily == "" {
				assert.Nil(t, si)
				return
			}
			require.NotNil(t, si)
			assert.Equal(t, tc.wantFamily, si.Family)
			if tc.wantPort != 0 {
				assert.Equal(t, tc.wantPort, si.Port)
			}
		})
	}
}

func TestScanDockerfilePort(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dockerfile string
		want       int
	}{
		{name: "EXPOSE", dockerfile: "FROM alpine\nEXPOSE 3000\n", want: 3000},
		{name: "first EXPOSE wins", dockerfile: "FROM alpine\nEXPOSE 4000\nEXPOSE 5000\n", want: 4000},
		{name: "no EXPOSE", dockerfile: "FROM alpine\n", want: 8080},
		{name: "commented out", dockerfile: "FROM alpine\n# EXPOSE 3000\n", want: 8080},
	} {
		t.Run(tc.name, func(t *testing.T) {
			si := scanFiles(t, map[string]string{"Dockerfile": tc.dockerfile})
			require.NotNil(t, si)
			assert.Equal(t, tc.want, si.Port)
		})
	}
}

func TestScanGo(t *testing.T) {
	t.Run("version from go.mod", func(t *testing.T) {
		si := scanFiles(t, map[string]string{"go.mod": "module x\n\ngo 1.23.4\n", "go.sum": ""})
		require.NotNil(t, si)

		assert.Equal(t, "1.23.4", si.BuildArgs["GO_VERSION"])
		assert.Equal(t, "go", si.Runtime.Language)
		assert.False(t, si.SkipDeploy)
	})

	// The generated Dockerfile runs go mod download, which needs go.sum.
	t.Run("no go.sum skips the deploy", func(t *testing.T) {
		si := scanFiles(t, map[string]string{"go.mod": "module x\n\ngo 1.22\n"})
		require.NotNil(t, si)

		assert.True(t, si.SkipDeploy)
	})

	t.Run("invalid go.mod still launches", func(t *testing.T) {
		si := scanFiles(t, map[string]string{"go.mod": "not a go.mod\n", "go.sum": ""})
		require.NotNil(t, si)

		assert.Equal(t, "1", si.BuildArgs["GO_VERSION"])
	})
}

func TestScanRustNeedsAPackage(t *testing.T) {
	t.Run("no [package]", func(t *testing.T) {
		t.Setenv("OPT_OUT_GITHUB_ACTIONS", "1")
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[workspace]\nmembers = [\"a\"]\n"), 0o644))
		t.Chdir(dir)

		_, err := Scan(dir, &ScannerConfig{Mode: "launch"})
		assert.ErrorContains(t, err, "does not contain a valid package section")
	})
}

func TestScanDjango(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		si := scanFiles(t, map[string]string{"requirements.txt": "Django==5.0\n"})
		require.NotNil(t, si)

		assert.Equal(t, DatabaseKindNone, si.DatabaseDesired)
		assert.False(t, si.RedisDesired)
		assert.False(t, si.ObjectStorageDesired)
		require.Len(t, si.Secrets, 1)
		assert.Equal(t, "SECRET_KEY", si.Secrets[0].Key)
		assert.NotNil(t, si.Secrets[0].Generate, "the secret is generated, not asked for")
	})

	t.Run("postgres, redis and S3 from dependencies", func(t *testing.T) {
		si := scanFiles(t, map[string]string{
			"requirements.txt": "Django==5.0\npsycopg2-binary\ndj-database-url\nredis\nboto3\n",
		})
		require.NotNil(t, si)

		assert.Equal(t, DatabaseKindPostgres, si.DatabaseDesired)
		assert.True(t, si.RedisDesired)
		assert.True(t, si.ObjectStorageDesired)
		assert.Equal(t, "python manage.py migrate --noinput", si.ReleaseCmd)
		assert.False(t, si.SkipDeploy)
	})

	// Without a way to read DATABASE_URL, the settings still point at
	// SQLite, so deploying would fail.
	t.Run("postgres without a DATABASE_URL reader skips the deploy", func(t *testing.T) {
		si := scanFiles(t, map[string]string{"requirements.txt": "Django==5.0\npsycopg2-binary\n"})
		require.NotNil(t, si)

		assert.Equal(t, DatabaseKindPostgres, si.DatabaseDesired)
		assert.True(t, si.SkipDeploy)
	})

	t.Run("Pipfile", func(t *testing.T) {
		si := scanFiles(t, map[string]string{"Pipfile": "[packages]\ndjango = \"*\"\n"})
		require.NotNil(t, si)

		assert.Equal(t, "Django", si.Family)
	})
}

func TestScanDotnet(t *testing.T) {
	csproj := func(framework string) map[string]string {
		return map[string]string{"app.csproj": `<Project Sdk="Microsoft.NET.Sdk.Web">
  <PropertyGroup>
    <TargetFramework>` + framework + `</TargetFramework>
  </PropertyGroup>
</Project>
`}
	}

	for _, tc := range []struct {
		framework string
		supported bool
	}{
		{framework: "net6.0", supported: true},
		{framework: "net8.0", supported: true},
		{framework: "net9.0", supported: true},
		{framework: "net10.0", supported: true},
		{framework: "netcoreapp3.1"},
		{framework: "net48"}, // .NET Framework
	} {
		t.Run(tc.framework, func(t *testing.T) {
			si := scanFiles(t, csproj(tc.framework))

			if !tc.supported {
				assert.Nil(t, si)
				return
			}
			require.NotNil(t, si)
			assert.Equal(t, ".NET", si.Family)
		})
	}
}
