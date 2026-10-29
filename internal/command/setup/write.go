package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tailscale/hujson"

	"github.com/superfly/flyctl/helpers"
)

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
}

func installTOML(path string, force bool) mcpOutcome {
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
		return failed(path, fmt.Sprintf("could not parse %s: %s", path, err.Error()))
	}

	return finishMerge(path, data, next, status)
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

// readConfig returns the file body. missing is set for a file that is not
// there or that is empty; an empty file has nothing to preserve, so it is
// created the same way as a missing one. Any other read error is returned
// and the caller must not write.
func readConfig(path string) (data []byte, missing bool, err error) {
	data, err = os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	info, statErr := os.Stat(path)
	if statErr == nil && info.IsDir() {
		return nil, false, fmt.Errorf("%s is a directory", path)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, true, nil
	}

	return data, false, nil
}

// writeConfig replaces path atomically and keeps the mode of an existing file.
func writeConfig(path string, data []byte) error {
	perm := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", path)
		}
		if mode := info.Mode().Perm(); mode != 0 {
			perm = mode
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return helpers.WriteFileAtomically(path, data, perm)
}

func configured(path string) mcpOutcome {
	return mcpOutcome{Status: statusConfigured, Path: path, Detail: "configured (" + path + ")"}
}

func already(path string) mcpOutcome {
	return mcpOutcome{Status: statusAlready, Path: path, Detail: "already configured (" + path + ")"}
}

func conflictOutcome(path string) mcpOutcome {
	return mcpOutcome{
		Status: statusConflict,
		Path:   path,
		Detail: "left unchanged (" + path + " points elsewhere; re-run with --force)",
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
