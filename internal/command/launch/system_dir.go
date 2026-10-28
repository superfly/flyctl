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
	// Of the plan steps, only generate writes into the working directory;
	// --manifest writes nothing there.
	if step := plan.GetPlanStep(ctx); (step != "" && step != "generate") || flag.GetBool(ctx, "manifest") {
		return nil
	}

	// The directory buildManifest works in.
	dir, err := filepath.Abs(flag.GetString(ctx, "path"))
	if err != nil {
		return nil
	}

	return checkWritable(dir, windowsSystemRoot())
}

// windowsSystemRoot is %SystemRoot%, or "" off Windows.
func windowsSystemRoot() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		return root
	}

	return os.Getenv("windir")
}

// checkWritable fails when dir exists but launch can't create files in it.
// A directory that doesn't exist is left for launch to report. Under
// systemRoot, the error says how people usually end up there.
func checkWritable(dir, systemRoot string) error {
	f, err := os.CreateTemp(dir, ".fly-launch-*")
	if err == nil {
		f.Close()
		os.Remove(f.Name())

		return nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	descript := fmt.Sprintf("fly launch writes fly.toml and other files into the directory it runs in, and can't create files here: %v", err)
	if isWithin(dir, systemRoot) {
		descript = "That's a Windows system folder, where a terminal opened as administrator starts. " + descript
	}

	return flyerr.GenericErr{
		Err:      fmt.Sprintf("fly launch can't write to %s", dir),
		Descript: descript,
		Suggest:  "cd into your project's directory and run fly launch again",
	}
}

// isWithin reports whether dir is root or inside it, ignoring case as
// Windows paths do.
func isWithin(dir, root string) bool {
	if root == "" {
		return false
	}
	dir = strings.ToLower(filepath.Clean(dir))
	root = strings.ToLower(filepath.Clean(root))

	return dir == root || strings.HasPrefix(dir, strings.TrimSuffix(root, string(filepath.Separator))+string(filepath.Separator))
}
