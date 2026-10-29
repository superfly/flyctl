package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tailscale/hujson"

	"github.com/superfly/flyctl/helpers"
)

// beforeWriteHook runs after the target has been resolved and checked, and
// before the bytes are published. Tests swap a symlink here to prove the
// write uses the path that was checked rather than resolving again.
var beforeWriteHook func()

// utf8BOM is the byte order mark some editors write at the start of a JSON file.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

const fileLockTimeout = 5 * time.Second

// mcpOutcome is what happened to one agent's MCP config.
type mcpOutcome struct {
	Status string
	Path   string
	Detail string
	Err    error
}

func configureMCP(home, slug string, force, skip bool) mcpOutcome {
	if skip {
		return mcpOutcome{Status: statusSkipped, Detail: "skipped (--skip-mcp)"}
	}
	client, ok := clientFor(slug)
	if !ok {
		return mcpOutcome{Status: statusSkipped, Detail: skillsOnlyDetail(slug)}
	}

	switch client.format {
	case formatJSON:
		return installJSON(home, client.path(home), client, force)
	case formatTOML:
		return installTOML(home, client.path(home), force)
	default:
		return mcpOutcome{Status: statusSkipped, Detail: skillsOnlyDetail(slug)}
	}
}

func installJSON(home, path string, client mcpClient, force bool) mcpOutcome {
	return withFileLock(home, path, func(wt writeTarget) mcpOutcome {
		data, missing, err := readConfig(wt.target)
		if err != nil {
			return failed(wt.logical, err.Error())
		}
		if missing {
			entry, err := client.entryJSON()
			if err != nil {
				return failed(wt.logical, err.Error())
			}
			fresh, err := freshJSON(client.container, entry)
			if err != nil {
				return failed(wt.logical, err.Error())
			}
			if err := writeConfig(wt, fresh); err != nil {
				return failed(wt.logical, err.Error())
			}

			return configured(wt)
		}

		next, status, err := mergeJSONBytes(data, client, force)
		if err != nil {
			if errors.Is(err, errManualEdit) {
				return failed(wt.logical, fmt.Sprintf("can't safely edit %s; edit the %q entry manually", wt.logical, mcpServerName))
			}

			return failed(wt.logical, fmt.Sprintf("could not parse %s: %s", wt.logical, err.Error()))
		}

		return finishMerge(wt, data, next, status)
	})
}

func installTOML(home, path string, force bool) mcpOutcome {
	return withFileLock(home, path, func(wt writeTarget) mcpOutcome {
		data, missing, err := readConfig(wt.target)
		if err != nil {
			return failed(wt.logical, err.Error())
		}
		if missing {
			if err := writeConfig(wt, []byte(codexTable("\n"))); err != nil {
				return failed(wt.logical, err.Error())
			}

			return configured(wt)
		}

		next, status, err := mergeCodexBytes(data, force)
		if err != nil {
			if errors.Is(err, errManualEdit) {
				return failed(wt.logical, fmt.Sprintf("can't safely edit %s; edit [mcp_servers.fly] manually", wt.logical))
			}

			return failed(wt.logical, fmt.Sprintf("could not parse %s: %s", wt.logical, err.Error()))
		}

		return finishMerge(wt, data, next, status)
	})
}

// writeTarget is the path resolved once while the config lock is held.
// target is the file the temp file is renamed onto. logical is the path the
// user sees. --force does not widen target.
type writeTarget struct {
	logical string
	target  string
	perm    os.FileMode
}

// withFileLock locks a file in the flyctl state directory, then resolves
// path once and refuses a target that escapes the expected directory.
// MkdirAll is only called for the expected directory, and only after the
// resolved target has been accepted.
func withFileLock(home, path string, fn func(writeTarget) mcpOutcome) mcpOutcome {
	unlock, err := acquireConfigLock(path)
	if err != nil {
		if errors.Is(err, errLockTimeout) {
			return failed(path, "timed out waiting to update "+path)
		}

		return failed(path, "could not lock "+path+": "+oneLine(err.Error()))
	}
	defer unlock()

	target, perm, err := resolveWriteTarget(path)
	if err != nil {
		return failed(path, err.Error())
	}
	if err := requireContained(home, path, target); err != nil {
		return failed(path, err.Error())
	}
	if err := mkdirExpected(path); err != nil {
		return failed(path, err.Error())
	}

	return fn(writeTarget{logical: path, target: target, perm: perm})
}

func finishMerge(wt writeTarget, original, next []byte, status string) mcpOutcome {
	switch status {
	case statusAlready:
		return already(wt)
	case statusConflict:
		return conflictOutcome(wt)
	case statusConfigured:
		if bytes.Equal(original, next) {
			return already(wt)
		}
		if len(next) == 0 {
			return failed(wt.logical, "refusing to write an empty config")
		}
		if err := writeConfig(wt, next); err != nil {
			return failed(wt.logical, err.Error())
		}

		return configured(wt)
	default:
		return failed(wt.logical, "could not update "+wt.logical)
	}
}

// readConfig returns the file body. missing is set when the file is not
// there, or when it is empty or whitespace-only. A file that is only a
// UTF-8 BOM, or a BOM plus whitespace, counts as missing too: there is
// nothing to preserve, so it is created the same way as a missing file.
// A BOM in front of real JSON is returned with the body so the write can
// keep it. Any other read error is returned and the caller must not write.
func readConfig(path string) (data []byte, missing bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	if info.IsDir() {
		return nil, false, fmt.Errorf("%s is a directory", path)
	}
	data, err = os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	body := bytes.TrimPrefix(data, utf8BOM)
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, true, nil
	}

	return data, false, nil
}

// writeConfig replaces the resolved file atomically and keeps its mode.
// wt.target was resolved once while the lock was held; this does not resolve
// again, so a symlink swapped after the check is not followed.
//
// A hard link is a second directory entry for the same inode. The rename
// that publishes the temp file replaces only the entry at the target path,
// so any other hard link keeps the previous contents. Writing through the
// inode would update every name, and would give up the atomic replace.
func writeConfig(wt writeTarget, data []byte) error {
	if beforeWriteHook != nil {
		beforeWriteHook()
	}
	restore, err := prepareReplace(wt.target)
	if err != nil {
		return err
	}
	defer restore()

	return helpers.WriteFileAtomically(wt.target, data, wt.perm)
}

// mkdirExpected creates the lexical parent of the config path. It does not
// create the directory of a resolved target that lives somewhere else. An
// existing directory, including one reached by a symlink, is left alone.
func mkdirExpected(logical string) error {
	dir := filepath.Dir(logical)
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		info, statErr := os.Stat(resolved)
		if statErr == nil && info.IsDir() {
			return nil
		}
	}

	return os.MkdirAll(dir, 0o755)
}

// requireContained refuses a resolved target that is not inside the expected
// config directory or the user's home. Both roots are compared in their
// lexical form and in their canonical form, because the target has already
// been canonicalized: a home behind a symlink (macOS /var -> /private/var,
// Fedora /home -> /var/home) or a Windows 8.3 short name must not look like an
// escape. The resolved client directory is allowed only when that directory
// itself stays inside home or the expected directory. A target under ~/.ssh
// is refused even though it is inside home, and --force is not consulted.
func requireContained(home, logical, resolved string) error {
	logical = filepath.Clean(logical)
	resolved = filepath.Clean(resolved)
	if home != "" && underSSH(home, resolved) {
		return fmt.Errorf("refusing to write %s: it resolves to %s, which is under %s", logical, resolved, filepath.Join(home, ".ssh"))
	}
	expected := filepath.Dir(logical)
	roots := []string{expected}
	if home != "" {
		roots = append(roots, home)
		if realHome := resolvePath(home); realHome != "" {
			roots = append(roots, realHome)
		}
	}
	for _, root := range roots {
		if pathWithin(resolved, root) {
			return nil
		}
	}
	// The expected directory may canonicalize differently from its lexical
	// form (an ancestor symlink, a short name). Accept a target inside the
	// canonical directory, unless the config directory itself is a symlink
	// that leaves every root: a ~/.cursor pointing outside home is refused.
	if realExpected := resolvePath(expected); realExpected != "" && pathWithin(resolved, realExpected) {
		if info, err := os.Lstat(expected); err == nil && info.Mode()&os.ModeSymlink != 0 {
			for _, root := range roots {
				if pathWithin(realExpected, root) {
					return nil
				}
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("refusing to write %s: it resolves to %s, which is outside the expected config directory", logical, resolved)
}

func underSSH(home, resolved string) bool {
	ssh := filepath.Join(home, ".ssh")
	if pathWithin(resolved, ssh) {
		return true
	}
	real, err := filepath.EvalSymlinks(ssh)
	if err != nil {
		return false
	}

	return pathWithin(resolved, real)
}

func pathWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	prefix := ".." + string(filepath.Separator)

	return rel != ".." && !strings.HasPrefix(rel, prefix)
}

// resolveWriteTarget returns the path to rename onto and the mode to keep.
// A missing path is created at path with mode 0644. A symlink is resolved
// all the way to the real file.
func resolveWriteTarget(path string) (target string, perm os.FileMode, err error) {
	perm = 0o644
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if danglingSymlink(path) {
			return "", 0, fmt.Errorf("%s is a symlink that cannot be resolved", path)
		}
		if resolved := resolvePath(path); resolved != "" {
			return resolved, perm, nil
		}

		return filepath.Clean(path), perm, nil
	}
	if err != nil {
		return "", 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", 0, fmt.Errorf("%s is a symlink that cannot be resolved", path)
		}
		targetInfo, err := os.Lstat(resolved)
		if err != nil {
			return "", 0, err
		}
		if targetInfo.IsDir() {
			return "", 0, fmt.Errorf("%s is a directory", path)
		}
		if mode := targetInfo.Mode().Perm(); mode != 0 {
			perm = mode
		}

		return resolved, perm, nil
	}
	if info.IsDir() {
		return "", 0, fmt.Errorf("%s is a directory", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", 0, err
	}
	if mode := info.Mode().Perm(); mode != 0 {
		perm = mode
	}

	return resolved, perm, nil
}

func danglingSymlink(path string) bool {
	cur := filepath.Dir(path)
	for {
		info, err := os.Lstat(cur)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			if _, evalErr := filepath.EvalSymlinks(cur); evalErr != nil {
				return true
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}

// annotatePath appends the resolved path when path, or a parent directory,
// is a symlink. The printed form is "path -> resolved".
func annotatePath(path string) string {
	if !pathHasSymlink(path) {
		return path
	}
	resolved := resolvePath(path)
	if resolved == "" || filepath.Clean(resolved) == filepath.Clean(path) {
		return path
	}

	return path + " -> " + resolved
}

func pathHasSymlink(path string) bool {
	cur := path
	for {
		info, err := os.Lstat(cur)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}

func resolvePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	var tail []string
	cur := path
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		tail = append(tail, filepath.Base(cur))
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}

			return resolved
		}
		cur = parent
	}
}

func showPath(wt writeTarget) string {
	if filepath.Clean(wt.target) == filepath.Clean(wt.logical) {
		return wt.logical
	}

	return wt.logical + " -> " + wt.target
}

func configured(wt writeTarget) mcpOutcome {
	return mcpOutcome{Status: statusConfigured, Path: wt.logical, Detail: "configured (" + showPath(wt) + ")"}
}

func already(wt writeTarget) mcpOutcome {
	return mcpOutcome{Status: statusAlready, Path: wt.logical, Detail: "already configured (" + showPath(wt) + ")"}
}

func conflictOutcome(wt writeTarget) mcpOutcome {
	shown := showPath(wt)
	return mcpOutcome{
		Status: statusConflict,
		Path:   wt.logical,
		Detail: "left unchanged (" + shown + " points elsewhere; re-run with --force)",
	}
}

func failed(path, msg string) mcpOutcome {
	return mcpOutcome{
		Status: statusFailed,
		Path:   path,
		Detail: "failed (" + oneLine(msg) + ")",
		Err:    errors.New(msg),
	}
}

// flyEntryFromFile returns the fly server object written for a JSON client.
func flyEntryFromFile(path, container string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(standardized, &root); err != nil {
		return nil, err
	}
	servers, ok := root[container].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is missing %s", path, container)
	}
	entry, ok := servers[mcpServerName].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is missing %s.%s", path, container, mcpServerName)
	}

	return entry, nil
}
