package skills

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// skillNameRe follows the agentskills.io naming rules: lowercase letters,
// digits and single hyphens, never leading or trailing.
var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const maxSkillNameLen = 64

// validSkillName reports whether name may be used as a skill directory name.
func validSkillName(name string) bool {
	return len(name) <= maxSkillNameLen && skillNameRe.MatchString(name)
}

// validRelPath reports whether rel is a safe forward-slashed path relative to
// a skill directory. The rules are the same on every OS: archive paths from
// GitHub are always forward-slashed, so a backslash, a colon, a NUL byte or
// a leading slash is never legitimate and would otherwise be interpreted by
// Windows as a separator, a drive letter or a UNC path.
func validRelPath(rel string) bool {
	if rel == "" || strings.ContainsAny(rel, "\\:\x00") || rel[0] == '/' {
		return false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	native := filepath.FromSlash(rel)
	if filepath.IsAbs(native) || filepath.VolumeName(native) != "" {
		return false
	}

	return true
}

var errUnsafePath = errors.New("unsafe path")

// safeJoin joins rel onto dir after validating it, and confirms as defense in
// depth that the result still lies inside dir.
func safeJoin(dir, rel string) (string, error) {
	if !validRelPath(rel) {
		return "", fmt.Errorf("%w: %q", errUnsafePath, rel)
	}
	joined := filepath.Join(dir, filepath.FromSlash(rel))
	inside, err := filepath.Rel(dir, joined)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) ||
		filepath.VolumeName(joined) != filepath.VolumeName(dir) {
		return "", fmt.Errorf("%w: %q escapes %s", errUnsafePath, rel, dir)
	}

	return joined, nil
}
