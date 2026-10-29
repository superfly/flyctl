package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// applyResult describes what applySkill did for one skill in one target.
type applyResult struct {
	Written []string // files created or overwritten
	Adopted []string // files already matching upstream, now tracked
	Removed []string // owned files upstream dropped, deleted
	// Skipped maps a relative path to why it was left alone: "modified",
	// "deleted locally" or "unverified local copy".
	Skipped map[string]string
}

func (r applyResult) skippedFiles() []string {
	return sortedKeys(r.Skipped)
}

func (r applyResult) changed() bool {
	return len(r.Written) > 0 || len(r.Removed) > 0
}

// applySkill writes one skill into skillDir file by file. A file is written
// when it is new, when the on-disk copy still matches what we recorded, or
// when it already matches upstream. A file the user modified or deleted, or
// an untracked file with different content, is left alone unless force is
// set. Owned files upstream dropped are deleted when unmodified.
//
// The returned record carries the new hash for every file written or
// adopted; skipped files keep the hash recorded before so a later run still
// tells a local edit from an upstream change.
func applySkill(skillDir string, files []SkillFile, rec SkillRecord, hasRec bool, sha string, force bool) (SkillRecord, applyResult, error) {
	res := applyResult{Skipped: map[string]string{}}
	next := SkillRecord{SourceSHA: sha, InstalledAt: nowRFC3339(), Files: map[string]string{}}

	// Retiring a linked skill must not prune files in the shared directory.
	if len(files) == 0 {
		if unlinked, err := unlinkSkillDir(skillDir); err != nil {
			return rec, res, err
		} else if unlinked {
			return next, res, nil
		}
	}

	sorted := append([]SkillFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Rel < sorted[j].Rel })

	for _, f := range sorted {
		path, err := safeJoin(skillDir, f.Rel)
		if err != nil {
			return rec, res, err
		}
		want := hashContent(f.Contents)
		recorded, owned := rec.Files[f.Rel]
		got, exists := hashDiskFile(path)

		switch {
		case exists && got == want:
			res.Adopted = append(res.Adopted, f.Rel)
			next.Files[f.Rel] = want
			continue
		case force:
		case !exists && owned && hasRec:
			res.Skipped[f.Rel] = "deleted locally"
			next.Files[f.Rel] = recorded
			continue
		case exists && owned && got != recorded:
			res.Skipped[f.Rel] = "modified"
			next.Files[f.Rel] = recorded
			continue
		case exists && !owned:
			res.Skipped[f.Rel] = "unverified local copy"
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return rec, res, fmt.Errorf("failed to create %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, f.Contents, 0o644); err != nil {
			return rec, res, fmt.Errorf("failed to write %s: %w", path, err)
		}
		res.Written = append(res.Written, f.Rel)
		next.Files[f.Rel] = want
	}

	// Files we wrote last time that upstream no longer ships.
	incoming := map[string]bool{}
	for _, f := range files {
		incoming[f.Rel] = true
	}
	for _, rel := range sortedKeys(rec.Files) {
		if incoming[rel] {
			continue
		}
		path, err := safeJoin(skillDir, rel)
		if err != nil {
			return rec, res, fmt.Errorf("manifest records an invalid path: %w", err)
		}
		got, exists := hashDiskFile(path)
		if !exists {
			continue
		}
		if got != rec.Files[rel] && !force {
			res.Skipped[rel] = "modified"
			next.Files[rel] = rec.Files[rel]
			continue
		}
		removed, err := removeSkillFile(skillDir, rel)
		if err != nil {
			return rec, res, err
		}
		if removed {
			res.Removed = append(res.Removed, rel)
		}
	}

	// The record's revision only moves when the whole skill is at it.
	if len(res.Skipped) > 0 {
		next.SourceSHA = rec.SourceSHA
		if !hasRec {
			next.SourceSHA = ""
		}
	}

	return next, res, nil
}

// removeSkillFiles deletes exactly the files a record lists, then any
// directories that became empty, up to and including skillDir. A recorded
// path that fails validation or traverses a symlink outside skillDir aborts
// the removal without deleting files outside skillDir. A linked
// skill directory is unlinked without deleting anything in its destination.
func removeSkillFiles(skillDir string, rec SkillRecord) (removed []string, err error) {
	if unlinked, err := unlinkSkillDir(skillDir); err != nil || unlinked {
		return nil, err
	}

	for _, rel := range sortedKeys(rec.Files) {
		deleted, err := removeSkillFile(skillDir, rel)
		if err != nil {
			return removed, err
		}
		if deleted {
			removed = append(removed, rel)
		}
	}
	// Remove the skill directory itself when nothing else lives there.
	_ = os.Remove(skillDir)

	return removed, nil
}

func unlinkSkillDir(skillDir string) (bool, error) {
	info, err := os.Lstat(skillDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to inspect %s: %w", skillDir, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	if err := os.Remove(skillDir); err != nil {
		return false, fmt.Errorf("failed to unlink %s: %w", skillDir, err)
	}

	return true, nil
}

// removeSkillFile confines both file deletion and empty-directory cleanup to
// the opened skill directory. Unlike lexical path validation, os.Root also
// prevents escapes through nested symlinks, including concurrent replacements.
func removeSkillFile(skillDir, rel string) (bool, error) {
	path, err := safeJoin(skillDir, rel)
	if err != nil {
		return false, fmt.Errorf("manifest records an invalid path: %w", err)
	}
	root, err := os.OpenRoot(skillDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to open %s: %w", skillDir, err)
	}
	defer root.Close() //nolint:errcheck

	native := filepath.FromSlash(rel)
	if err := root.Remove(native); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to remove %s: %w", path, err)
	}
	removeEmptyParents(root, filepath.Dir(native))

	return true, nil
}

// removeEmptyParents removes directories relative to root while they are empty.
func removeEmptyParents(root *os.Root, dir string) {
	for dir != "." {
		if err := root.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
