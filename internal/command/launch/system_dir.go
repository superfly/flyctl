package launch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
)

// checkLaunchDir stops a launch into a directory it can't write fly.toml and
// the rest into, before it creates anything. The common case is the Windows
// system folder, where a terminal opened as administrator starts: launching
// there used to fail late with "open C:\WINDOWS\system32\fly.toml: Access is
// denied". Call it after setupFromTemplate, which changes into --into.
func checkLaunchDir(ctx context.Context) error {
	// Plan steps and --manifest write nothing into the working directory.
	if plan.GetPlanStep(ctx) != "" || flag.GetBool(ctx, "manifest") {
		return nil
	}

	// The directory buildManifest works in.
	dir, err := filepath.Abs(flag.GetString(ctx, "path"))
	if err != nil {
		return nil
	}

	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = os.Getenv("windir")
		}
		if err := checkNotInSystemDir(canonicalPath(dir), canonicalPath(systemRoot)); err != nil {
			return err
		}
	}

	return checkWritable(dir)
}

// canonicalPath resolves symlinks and junctions, which on Windows also
// expands 8.3 short names (C:\WINDOW~1), so different spellings of a
// directory compare equal. A path that doesn't resolve is returned as is.
func canonicalPath(p string) string {
	if p == "" {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}

	return p
}

// checkWritable fails when dir exists but launch can't create files in it.
// A directory that doesn't exist is left for launch to report.
func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".fly-launch-*")
	if err != nil {
		if !errors.Is(err, fs.ErrPermission) {
			return nil
		}

		return flyerr.GenericErr{
			Err:      fmt.Sprintf("fly launch can't write to %s", dir),
			Descript: "fly launch writes fly.toml and other files into the directory it runs in.",
			Suggest:  "Run fly launch in your project's directory, or give yourself write access to this one",
		}
	}
	f.Close()
	os.Remove(f.Name())

	return nil
}

// checkNotInSystemDir fails when dir is systemRoot or inside it. Windows paths
// are case-insensitive, and either slash separates them.
func checkNotInSystemDir(dir, systemRoot string) error {
	normalize := func(p string) string {
		p = strings.TrimPrefix(strings.ReplaceAll(p, "/", `\`), `\\?\`)

		return strings.TrimRight(strings.ToLower(p), `\`)
	}

	root := normalize(systemRoot)
	// A system folder at the root of a drive (or none at all) would match
	// every directory on it.
	if root == "" || strings.HasSuffix(root, ":") {
		return nil
	}

	if d := normalize(dir); d != root && !strings.HasPrefix(d, root+`\`) {
		return nil
	}

	return flyerr.GenericErr{
		Err:      fmt.Sprintf("fly launch can't run in %s", dir),
		Descript: "That's a Windows system folder, where a terminal opened as administrator starts. fly launch writes fly.toml and other files into the directory it runs in.",
		Suggest:  "cd into your project's directory and run fly launch again",
	}
}
