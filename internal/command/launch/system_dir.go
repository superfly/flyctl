package launch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
)

// checkLaunchDir stops a launch into the Windows system folder before it gets
// as far as "open C:\WINDOWS\system32\fly.toml: Access is denied". A terminal
// opened as administrator starts there, so people run fly launch in it by
// accident. Call it after setupFromTemplate, which changes into --into.
func checkLaunchDir(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		return nil
	}

	// Plan steps and --manifest write nothing into the working directory.
	if plan.GetPlanStep(ctx) != "" || flag.GetBool(ctx, "manifest") {
		return nil
	}

	// The directory buildManifest works in.
	dir, err := filepath.Abs(flag.GetString(ctx, "path"))
	if err != nil {
		return nil
	}

	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = os.Getenv("windir")
	}

	return checkNotInSystemDir(dir, systemRoot)
}

// checkNotInSystemDir fails when dir is systemRoot or inside it. Windows paths
// are case-insensitive, and either slash separates them.
func checkNotInSystemDir(dir, systemRoot string) error {
	normalize := func(p string) string {
		return strings.TrimRight(strings.ToLower(strings.ReplaceAll(p, "/", `\`)), `\`)
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
