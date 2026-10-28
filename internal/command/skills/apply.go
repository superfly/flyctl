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

	sorted := append([]SkillFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Rel < sorted[j].Rel })

	for _, f := range sorted {
		path := joinRel(skillDir, f.Rel)
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
		path := joinRel(skillDir, rel)
		got, exists := hashDiskFile(path)
		if !exists {
			continue
		}
		if got != rec.Files[rel] && !force {
			res.Skipped[rel] = "modified"
			next.Files[rel] = rec.Files[rel]
			continue
		}
		if err := os.Remove(path); err != nil {
			return rec, res, fmt.Errorf("failed to remove %s: %w", path, err)
		}
		res.Removed = append(res.Removed, rel)
		removeEmptyParents(filepath.Dir(path), skillDir)
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
// directories that became empty, up to and including skillDir.
func removeSkillFiles(skillDir string, rec SkillRecord) (removed []string, err error) {
	for _, rel := range sortedKeys(rec.Files) {
		path := joinRel(skillDir, rel)
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, fmt.Errorf("failed to remove %s: %w", path, err)
		}
		removed = append(removed, rel)
		removeEmptyParents(filepath.Dir(path), skillDir)
	}
	// Remove the skill directory itself when nothing else lives there.
	_ = os.Remove(skillDir)

	return removed, nil
}

// removeEmptyParents removes dir and its ancestors while they are empty,
// stopping before stop.
func removeEmptyParents(dir, stop string) {
	for dir != stop && len(dir) > len(stop) {
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
