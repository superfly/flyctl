package launch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
)

// checkLaunchDir stops a launch into the Windows system folder before it gets
// as far as "open C:\WINDOWS\system32\fly.toml: Access is denied". A terminal
// opened as administrator starts there, so people run fly launch in it by
// accident.
func checkLaunchDir(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		return nil
	}

	dir := flag.GetString(ctx, "into")
	if dir == "" {
		dir = flag.GetString(ctx, "path")
	}
	if dir == "" {
		dir = "."
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}

	return checkNotInSystemDir(abs, os.Getenv("SystemRoot"))
}

// checkNotInSystemDir fails when dir is systemRoot or inside it. Windows paths
// are case-insensitive, and either slash separates them.
func checkNotInSystemDir(dir, systemRoot string) error {
	normalize := func(p string) string {
		return strings.TrimRight(strings.ToLower(strings.ReplaceAll(p, "/", `\`)), `\`)
	}

	root := normalize(systemRoot)
	if root == "" {
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
