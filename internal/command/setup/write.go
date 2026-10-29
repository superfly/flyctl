package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/tailscale/hujson"

	"github.com/superfly/flyctl/helpers"
)

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
		return installJSON(client.path(home), client, force)
	case formatTOML:
		return installTOML(client.path(home), force)
	default:
		return mcpOutcome{Status: statusSkipped, Detail: skillsOnlyDetail(slug)}
	}
}

func installJSON(path string, client mcpClient, force bool) mcpOutcome {
	return withFileLock(path, func() mcpOutcome {
		data, missing, err := readConfig(path)
		if err != nil {
			return failed(path, err.Error())
		}
		if missing {
			entry, err := client.entryJSON()
			if err != nil {
				return failed(path, err.Error())
			}
			fresh, err := freshJSON(client.container, entry)
			if err != nil {
				return failed(path, err.Error())
			}
			if err := writeConfig(path, fresh); err != nil {
				return failed(path, err.Error())
			}

			return configured(path)
		}

		next, status, err := mergeJSONBytes(data, client, force)
		if err != nil {
			return failed(path, fmt.Sprintf("could not parse %s: %s", path, err.Error()))
		}

		return finishMerge(path, data, next, status)
	})
}

func installTOML(path string, force bool) mcpOutcome {
	return withFileLock(path, func() mcpOutcome {
		data, missing, err := readConfig(path)
		if err != nil {
			return failed(path, err.Error())
		}
		if missing {
			if err := writeConfig(path, []byte(codexTable("\n"))); err != nil {
				return failed(path, err.Error())
			}

			return configured(path)
		}

		next, status, err := mergeCodexBytes(data, force)
		if err != nil {
			if errors.Is(err, errManualEdit) {
				return failed(path, fmt.Sprintf("can't safely edit %s; edit [mcp_servers.fly] manually", path))
			}

			return failed(path, fmt.Sprintf("could not parse %s: %s", path, err.Error()))
		}

		return finishMerge(path, data, next, status)
	})
}

// withFileLock takes an exclusive lock on the resolved config, then re-reads
// and writes inside that lock so two setup processes cannot clobber each other.
func withFileLock(path string, fn func() mcpOutcome) mcpOutcome {
	target, _, err := resolveWriteTarget(path)
	if err != nil {
		return failed(path, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return failed(path, err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), fileLockTimeout)
	defer cancel()

	mu := flock.New(target + ".lock")
	locked, err := mu.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil || !locked {
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return failed(path, "could not lock "+path+": "+oneLine(err.Error()))
		}

		return failed(path, "timed out waiting to update "+path)
	}
	defer func() { _ = mu.Unlock() }()

	return fn()
}

func finishMerge(path string, original, next []byte, status string) mcpOutcome {
	switch status {
	case statusAlready:
		return already(path)
	case statusConflict:
		return conflictOutcome(path)
	case statusConfigured:
		if bytes.Equal(original, next) {
			return already(path)
		}
		if len(next) == 0 {
			return failed(path, "refusing to write an empty config")
		}
		if err := writeConfig(path, next); err != nil {
			return failed(path, err.Error())
		}

		return configured(path)
	default:
		return failed(path, "could not update "+path)
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

// writeConfig replaces the real file atomically and keeps its mode.
//
// Lstat is used so a symlink is not mistaken for the file it points at.
// When path is a symlink, the temp file is renamed onto the resolved target
// and the link itself is left in place. The mode comes from that target.
// A dangling symlink is an error; it is not replaced with a regular file.
//
// A hard link is a second directory entry for the same inode. The rename
// that publishes the temp file replaces only the entry at the target path,
// so any other hard link keeps the previous contents. Writing through the
// inode would update every name, and would give up the atomic replace.
func writeConfig(path string, data []byte) error {
	target, perm, err := resolveWriteTarget(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	restore, err := prepareReplace(target)
	if err != nil {
		return err
	}
	defer restore()

	return helpers.WriteFileAtomically(target, data, perm)
}

// resolveWriteTarget returns the path to rename onto and the mode to keep.
// A missing path is created at path with mode 0644. A symlink is resolved
// all the way to the real file.
func resolveWriteTarget(path string) (target string, perm os.FileMode, err error) {
	perm = 0o644
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, perm, nil
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
	if mode := info.Mode().Perm(); mode != 0 {
		perm = mode
	}

	return path, perm, nil
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

func configured(path string) mcpOutcome {
	return mcpOutcome{Status: statusConfigured, Path: path, Detail: "configured (" + annotatePath(path) + ")"}
}

func already(path string) mcpOutcome {
	return mcpOutcome{Status: statusAlready, Path: path, Detail: "already configured (" + annotatePath(path) + ")"}
}

func conflictOutcome(path string) mcpOutcome {
	shown := annotatePath(path)
	return mcpOutcome{
		Status: statusConflict,
		Path:   path,
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
