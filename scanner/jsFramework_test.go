package scanner

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configureJsFramework runs node to check its version, so these tests need it.
func requireNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
}

func TestScanJsFrameworkWithoutStartScript(t *testing.T) {
	requireNode(t)

	for _, tc := range []struct {
		name        string
		files       map[string]string
		wantFamily  string
		wantPort    int
		wantScanned bool
	}{
		// The package.json of the minimal template from npm create astro@latest (Astro 5),
		// which has no start script.
		{
			name: "Astro static site",
			files: map[string]string{
				"package.json": `{
  "name": "astro-site",
  "type": "module",
  "version": "0.0.1",
  "scripts": {"dev": "astro dev", "build": "astro build", "preview": "astro preview", "astro": "astro"},
  "dependencies": {"astro": "^5.16.10"}
}`,
				"src/pages/index.astro": "<h1>hi</h1>\n",
			},
			wantFamily: "Astro Static Site", wantPort: 80, wantScanned: true,
		},
		{
			name: "Astro with SSR",
			files: map[string]string{
				"package.json": `{
  "name": "astro-ssr",
  "type": "module",
  "scripts": {"dev": "astro dev", "build": "astro build", "preview": "astro preview"},
  "dependencies": {"astro": "^5.16.10", "@astrojs/node": "^9.5.0"}
}`,
			},
			wantFamily: "Astro with SSR", wantPort: 4321, wantScanned: true,
		},
		// A package.json that only serves a static site in development is left to
		// the static scanner.
		{
			name: "static site with a dev server",
			files: map[string]string{
				"package.json":      `{"name": "site", "type": "module", "scripts": {"dev": "serve public"}, "devDependencies": {"serve": "^14.0.0"}}`,
				"public/index.html": "<h1>hi</h1>\n",
			},
			wantFamily: "Static", wantPort: 8080, wantScanned: true,
		},
		{
			name: "built site with a Vite dev server",
			files: map[string]string{
				"package.json":    `{"name": "vite-site", "type": "module", "scripts": {"dev": "vite"}, "devDependencies": {"vite": "^7.0.0"}}`,
				"dist/index.html": "<h1>hi</h1>\n",
			},
			wantFamily: "Static", wantPort: 8080, wantScanned: true,
		},
		{
			name: "node app without a start script",
			files: map[string]string{
				"package.json": `{"name": "app", "main": "index.js", "dependencies": {"express": "^4.0.0"}}`,
				"index.js":     "require('express')().listen(3000)\n",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			si := scanFiles(t, tc.files)

			if !tc.wantScanned {
				assert.Nil(t, si)
				return
			}
			require.NotNil(t, si)
			assert.Equal(t, tc.wantFamily, si.Family)
			assert.Equal(t, tc.wantPort, si.Port)
		})
	}
}
