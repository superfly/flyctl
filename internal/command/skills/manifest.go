package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/superfly/flyctl/helpers"
)

// manifestFileName is the manifest's name inside flyctl's config directory
// (normally ~/.fly). It lives there rather than in the skill directories so
// coding tools that enumerate those directories never see a stray file.
const manifestFileName = "skills.json"

// SkillRecord is what we last wrote for one skill into one target.
type SkillRecord struct {
	// SourceSHA is the skills repository commit the files came from. It only
	// advances when every incoming file was applied.
	SourceSHA   string `json:"source_sha,omitempty"`
	InstalledAt string `json:"installed_at"`
	// Files maps a forward-slashed path relative to the skill directory to
	// the content hash we wrote.
	Files map[string]string `json:"files"`
}

// Manifest records what fly skills has installed, keyed by target skills
// directory and then by skill name.
type Manifest struct {
	// SourceSHA is the last revision that was applied in full everywhere.
	SourceSHA string `json:"source_sha,omitempty"`
	// ArchiveSHA256 is the hex digest of the tarball that revision was
	// installed from, recorded together with SourceSHA.
	ArchiveSHA256 string `json:"archive_sha256,omitempty"`
	// SourceRef is the branch, tag or commit the install is pinned to. It is
	// empty when tracking main. Later update and status runs use it unless
	// --ref or FLY_SKILLS_REF overrides it.
	SourceRef string `json:"source_ref,omitempty"`
	// LatestSHA is the newest upstream revision seen, applied or not.
	LatestSHA   string                            `json:"latest_sha,omitempty"`
	LastChecked string                            `json:"last_checked,omitempty"`
	Targets     map[string]map[string]SkillRecord `json:"targets"`
}

func manifestPath(configDir string) string {
	return filepath.Join(configDir, manifestFileName)
}

// readManifest loads the manifest. A missing file is an empty manifest; a
// corrupt one is an error so we never silently forget what we installed.
func readManifest(configDir string) (*Manifest, error) {
	m := &Manifest{Targets: map[string]map[string]SkillRecord{}}

	data, err := os.ReadFile(manifestPath(configDir))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, err
	}
	if m.Targets == nil {
		m.Targets = map[string]map[string]SkillRecord{}
	}

	return m, nil
}

func (m *Manifest) save(configDir string) error {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return helpers.WriteFileAtomically(manifestPath(configDir), append(data, '\n'), 0o600)
}

func (m *Manifest) record(targetKey, skill string) (SkillRecord, bool) {
	rec, ok := m.Targets[targetKey][skill]
	return rec, ok
}

func (m *Manifest) setRecord(targetKey, skill string, rec SkillRecord) {
	if m.Targets[targetKey] == nil {
		m.Targets[targetKey] = map[string]SkillRecord{}
	}
	m.Targets[targetKey][skill] = rec
}

func (m *Manifest) deleteRecord(targetKey, skill string) {
	delete(m.Targets[targetKey], skill)
	if len(m.Targets[targetKey]) == 0 {
		delete(m.Targets, targetKey)
	}
}

func (m *Manifest) hasInstalledSkills() bool {
	for _, skills := range m.Targets {
		if len(skills) > 0 {
			return true
		}
	}

	return false
}

// targetRevision summarises the revision a target is at: the common SHA when
// every skill agrees, "" when nothing is recorded, and mixed=true otherwise.
func (m *Manifest) targetRevision(targetKey string) (sha string, mixed bool) {
	for i, name := range sortedKeys(m.Targets[targetKey]) {
		rec := m.Targets[targetKey][name]
		switch {
		case i == 0:
			sha = rec.SourceSHA
		case rec.SourceSHA != sha:
			return sha, true
		}
	}

	return sha, false
}

// hashContent hashes file contents with carriage returns stripped, so a CRLF
// rewrite by an editor or git does not read as a user modification.
func hashContent(data []byte) string {
	normalized := make([]byte, 0, len(data))
	for _, b := range data {
		if b != '\r' {
			normalized = append(normalized, b)
		}
	}
	sum := sha256.Sum256(normalized)

	return hex.EncodeToString(sum[:])
}

// hashDiskFile returns the hash of a file on disk, or ok=false when it does
// not exist or cannot be read.
func hashDiskFile(path string) (hash string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return hashContent(data), true
}

func newFileHashes(files []SkillFile) map[string]string {
	hashes := make(map[string]string, len(files))
	for _, f := range files {
		hashes[f.Rel] = hashContent(f.Contents)
	}
	return hashes
}

// joinRel joins a forward-slashed path onto dir without validation. Use
// safeJoin for anything read from an archive or the manifest.
func joinRel(dir, rel string) string {
	return filepath.Join(dir, filepath.FromSlash(rel))
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// SkillState is how an on-disk skill compares to the recorded baseline and
// the incoming upstream content.
type SkillState int

const (
	// NotInstalled means the skill directory does not exist.
	NotInstalled SkillState = iota
	// UpToDate means every incoming file is already on disk with matching
	// content and nothing we own has been left behind.
	UpToDate
	// CleanUpgrade means every file we own is unmodified and upstream moved.
	CleanUpgrade
	// Modified means at least one file we own was edited or deleted, or a
	// user-added file collides with an incoming one.
	Modified
	// Unverifiable means the directory exists but we have no baseline and it
	// differs from upstream, so we cannot prove it is untouched.
	Unverifiable
)

func (s SkillState) String() string {
	switch s {
	case NotInstalled:
		return "not installed"
	case UpToDate:
		return "up to date"
	case CleanUpgrade:
		return "update available"
	case Modified:
		return "modified"
	case Unverifiable:
		return "unverifiable"
	}

	return "unknown"
}

// classifySkill compares skillDir against the incoming hashes and the baseline
// we recorded last time (rec, when ok).
func classifySkill(skillDir string, newHashes map[string]string, rec SkillRecord, ok bool) SkillState {
	if !isDir(skillDir) {
		return NotInstalled
	}

	owned := newHashes
	if ok {
		owned = rec.Files
	}

	disk := map[string]*string{}
	hashOnDisk := func(rel string) *string {
		if h, seen := disk[rel]; seen {
			return h
		}
		var h *string
		if p, err := safeJoin(skillDir, rel); err == nil {
			if v, found := hashDiskFile(p); found {
				h = &v
			}
		}
		disk[rel] = h

		return h
	}

	matchesNew := true
	for rel, want := range newHashes {
		if got := hashOnDisk(rel); got == nil || *got != want {
			matchesNew = false
			break
		}
	}
	lingering := false
	for rel := range owned {
		if _, incoming := newHashes[rel]; !incoming && hashOnDisk(rel) != nil {
			lingering = true
			break
		}
	}
	if matchesNew && !lingering {
		return UpToDate
	}

	if !ok {
		return Unverifiable
	}

	for rel, recorded := range rec.Files {
		if got := hashOnDisk(rel); got == nil || *got != recorded {
			return Modified
		}
	}
	for rel, want := range newHashes {
		if _, recorded := rec.Files[rel]; recorded {
			continue
		}
		if got := hashOnDisk(rel); got != nil && *got != want {
			return Modified
		}
	}

	return CleanUpgrade
}

// modifiedFiles lists the recorded files whose on-disk content no longer
// matches the baseline, including files that were deleted.
func modifiedFiles(skillDir string, rec SkillRecord) []string {
	var out []string
	for _, rel := range sortedKeys(rec.Files) {
		p, err := safeJoin(skillDir, rel)
		if err != nil {
			out = append(out, rel)

			continue
		}
		if got, ok := hashDiskFile(p); !ok || got != rec.Files[rel] {
			out = append(out, rel)
		}
	}
	sort.Strings(out)

	return out
}
