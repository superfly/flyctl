package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tarEntry struct {
	name string
	body string
	dir  bool
}

// makeTarball builds a gzipped tarball shaped like a GitHub archive: every
// entry lives under a "<repo>-<ref>/" top directory.
func makeTarball(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		if e.dir {
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0o755
			hdr.Size = 0
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if !e.dir {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	return buf.Bytes()
}

func upstreamTarball(t *testing.T) []byte {
	t.Helper()
	return makeTarball(t, []tarEntry{
		{name: "skills-abc123/", dir: true},
		{name: "skills-abc123/README.md", body: "# repo readme, not a skill\n"},
		{name: "skills-abc123/plugins/fly/.claude-plugin/plugin.json", body: "{}"},
		{name: "skills-abc123/plugins/fly/skills/", dir: true},
		{name: "skills-abc123/plugins/fly/skills/fly/", dir: true},
		{name: "skills-abc123/plugins/fly/skills/fly/SKILL.md", body: "# Fly\n"},
		{name: "skills-abc123/plugins/fly/skills/fly/references/deploy.md", body: "deploy\n"},
		{name: "skills-abc123/plugins/fly/skills/brand/SKILL.md", body: "# Brand\n"},
		{name: "skills-abc123/notplugins/fly/skills/evil/SKILL.md", body: "wrong prefix\n"},
		{name: "skills-abc123/plugins/fly/skills/traversal/../../../escape.md", body: "escape\n"},
		{name: "skills-abc123/plugins/fly/skills/.hidden/SKILL.md", body: "hidden\n"},
		{name: "skills-abc123/plugins/fly/skills/loose-file.md", body: "no skill dir\n"},
	})
}

func TestExtractSkillFiles(t *testing.T) {
	skills, err := extractSkillFiles(upstreamTarball(t))
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"fly", "brand"}, sortedKeys(skills))

	fly := map[string]string{}
	for _, f := range skills["fly"] {
		fly[f.Rel] = string(f.Contents)
	}
	assert.Equal(t, map[string]string{
		"SKILL.md":             "# Fly\n",
		"references/deploy.md": "deploy\n",
	}, fly)

	require.Len(t, skills["brand"], 1)
	assert.Equal(t, "SKILL.md", skills["brand"][0].Rel)
	assert.Equal(t, "# Brand\n", string(skills["brand"][0].Contents))
}

func TestExtractSkillFilesNoSkills(t *testing.T) {
	tarball := makeTarball(t, []tarEntry{
		{name: "skills-abc123/README.md", body: "nothing here\n"},
	})
	_, err := extractSkillFiles(tarball)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no skills found")
}

func TestExtractSkillFilesNotGzip(t *testing.T) {
	_, err := extractSkillFiles([]byte("not a tarball"))
	require.Error(t, err)
}

func TestSplitSkillPath(t *testing.T) {
	cases := []struct {
		entry    string
		name     string
		rel      string
		expectOK bool
	}{
		{"skills-main/plugins/fly/skills/fly/SKILL.md", "fly", "SKILL.md", true},
		{"./skills-main/plugins/fly/skills/fly/a/b.md", "fly", "a/b.md", true},
		{"plugins/fly/skills/brand/SKILL.md", "brand", "SKILL.md", true},
		{"skills-main/plugins/fly/skills/fly/", "", "", false},
		{"skills-main/plugins/fly/skills/fly", "", "", false},
		{"skills-main/plugins/fly/README.md", "", "", false},
		{"skills-main/xplugins/fly/skills/fly/SKILL.md", "", "", false},
		{"skills-main/plugins/fly/skills/fly/../../x.md", "", "", false},
		{"skills-main/plugins/fly/skills/fly/a/../b.md", "", "", false},
		{"skills-main/plugins/fly/skills/../fly/SKILL.md", "", "", false},
		// Backslashes are never legitimate in a GitHub archive and would be
		// separators on Windows.
		{`skills-main/plugins/fly/skills/fly/..\..\x`, "", "", false},
		{`skills-main/plugins/fly/skills/fly/a\..\..\x`, "", "", false},
		{`skills-main/plugins/fly/skills/fly/a/..\b`, "", "", false},
		{`skills-main/plugins/fly/skills/fly/C:\x`, "", "", false},
		{`skills-main/plugins/fly/skills/fly/C:x`, "", "", false},
		{`skills-main/plugins/fly/skills/fly/\\server\share`, "", "", false},
		{"skills-main/plugins/fly/skills/fly/a\x00b", "", "", false},
		{"skills-main/plugins/fly/skills/fly//etc/passwd", "", "", false},
		// Skill names follow the agentskills.io rules.
		{`skills-main/plugins/fly/skills/fly\../SKILL.md`, "", "", false},
		{"skills-main/plugins/fly/skills/Fly/SKILL.md", "", "", false},
		{"skills-main/plugins/fly/skills/-fly/SKILL.md", "", "", false},
		{"skills-main/plugins/fly/skills/fly--x/SKILL.md", "", "", false},
		{"skills-main/plugins/fly/skills/.hidden/SKILL.md", "", "", false},
		{"skills-main/plugins/fly/skills/C:/SKILL.md", "", "", false},
		{"skills-main/plugins/fly/skills/fly-2/SKILL.md", "fly-2", "SKILL.md", true},
	}
	for _, tc := range cases {
		name, rel, ok := splitSkillPath(tc.entry, skillsPathPrefix)
		assert.Equal(t, tc.expectOK, ok, tc.entry)
		assert.Equal(t, tc.name, name, tc.entry)
		assert.Equal(t, tc.rel, rel, tc.entry)
	}
}

func TestHashContentIgnoresCarriageReturns(t *testing.T) {
	assert.Equal(t, hashContent([]byte("a\nb\n")), hashContent([]byte("a\r\nb\r\n")))
	assert.NotEqual(t, hashContent([]byte("a\nb\n")), hashContent([]byte("a\nb")))
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := joinRel(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
}

func skillFiles(files map[string]string) []SkillFile {
	var out []SkillFile
	for _, rel := range sortedKeys(files) {
		out = append(out, SkillFile{Rel: rel, Contents: []byte(files[rel])})
	}
	return out
}

func recordFor(files map[string]string, sha string) SkillRecord {
	rec := SkillRecord{SourceSHA: sha, InstalledAt: "2026-01-01T00:00:00Z", Files: map[string]string{}}
	for rel, body := range files {
		rec.Files[rel] = hashContent([]byte(body))
	}
	return rec
}

func TestClassifySkill(t *testing.T) {
	v1 := map[string]string{"SKILL.md": "v1\n", "references/a.md": "a\n"}
	v2 := map[string]string{"SKILL.md": "v2\n", "references/b.md": "b\n"}

	t.Run("not installed", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		assert.Equal(t, NotInstalled, classifySkill(dir, newFileHashes(skillFiles(v1)), SkillRecord{}, false))
	})

	t.Run("up to date", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		assert.Equal(t, UpToDate, classifySkill(dir, newFileHashes(skillFiles(v1)), recordFor(v1, "s1"), true))
	})

	t.Run("up to date without a record", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		assert.Equal(t, UpToDate, classifySkill(dir, newFileHashes(skillFiles(v1)), SkillRecord{}, false))
	})

	t.Run("lingering dropped file is not up to date", func(t *testing.T) {
		// Upstream drops references/a.md but leaves SKILL.md as it was, and
		// the user already has references/b.md: every incoming file matches,
		// yet the dropped file still lingers, so an upgrade is due.
		dropped := map[string]string{"SKILL.md": "v1\n", "references/b.md": "b\n"}
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		writeFiles(t, dir, map[string]string{"references/b.md": "b\n"})
		assert.Equal(t, CleanUpgrade, classifySkill(dir, newFileHashes(skillFiles(dropped)), recordFor(v1, "s1"), true))
	})

	t.Run("clean upgrade", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		assert.Equal(t, CleanUpgrade, classifySkill(dir, newFileHashes(skillFiles(v2)), recordFor(v1, "s1"), true))
	})

	t.Run("modified file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		writeFiles(t, dir, map[string]string{"SKILL.md": "my edits\n"})
		assert.Equal(t, Modified, classifySkill(dir, newFileHashes(skillFiles(v2)), recordFor(v1, "s1"), true))
		assert.Equal(t, []string{"SKILL.md"}, modifiedFiles(dir, recordFor(v1, "s1")))
	})

	t.Run("deleted file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		require.NoError(t, os.Remove(joinRel(dir, "references/a.md")))
		assert.Equal(t, Modified, classifySkill(dir, newFileHashes(skillFiles(v2)), recordFor(v1, "s1"), true))
		assert.Equal(t, []string{"references/a.md"}, modifiedFiles(dir, recordFor(v1, "s1")))
	})

	t.Run("user file collides with incoming", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		writeFiles(t, dir, map[string]string{"references/b.md": "mine\n"})
		assert.Equal(t, Modified, classifySkill(dir, newFileHashes(skillFiles(v2)), recordFor(v1, "s1"), true))
	})

	t.Run("unverifiable without a record", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		assert.Equal(t, Unverifiable, classifySkill(dir, newFileHashes(skillFiles(v2)), SkillRecord{}, false))
	})
}

func TestApplySkill(t *testing.T) {
	v1 := map[string]string{"SKILL.md": "v1\n", "references/a.md": "a\n"}
	v2 := map[string]string{"SKILL.md": "v2\n", "references/b.md": "b\n"}

	readAll := func(t *testing.T, dir string) map[string]string {
		t.Helper()
		out := map[string]string{}
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dir, path)
			body, _ := os.ReadFile(path)
			out[filepath.ToSlash(rel)] = string(body)

			return nil
		})

		return out
	}

	t.Run("fresh install writes everything", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		rec, res, err := applySkill(dir, skillFiles(v1), SkillRecord{}, false, "s1", false)
		require.NoError(t, err)
		assert.Equal(t, v1, readAll(t, dir))
		assert.ElementsMatch(t, []string{"SKILL.md", "references/a.md"}, res.Written)
		assert.Empty(t, res.Skipped)
		assert.Equal(t, "s1", rec.SourceSHA)
		assert.Equal(t, recordFor(v1, "s1").Files, rec.Files)
	})

	t.Run("clean upgrade overwrites and prunes dropped files", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		rec, res, err := applySkill(dir, skillFiles(v2), recordFor(v1, "s1"), true, "s2", false)
		require.NoError(t, err)
		assert.Equal(t, v2, readAll(t, dir))
		assert.Equal(t, []string{"references/a.md"}, res.Removed)
		assert.Equal(t, "s2", rec.SourceSHA)
		assert.Equal(t, recordFor(v2, "s2").Files, rec.Files)
	})

	t.Run("modified file is kept and revision does not advance", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		writeFiles(t, dir, map[string]string{"SKILL.md": "my edits\n"})
		rec, res, err := applySkill(dir, skillFiles(v2), recordFor(v1, "s1"), true, "s2", false)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"SKILL.md": "my edits\n", "references/b.md": "b\n"}, readAll(t, dir))
		assert.Equal(t, map[string]string{"SKILL.md": "modified"}, res.Skipped)
		assert.Equal(t, "s1", rec.SourceSHA, "revision stays until the whole skill is at it")
		// The kept file keeps its old baseline so the next run still sees it as modified.
		assert.Equal(t, hashContent([]byte("v1\n")), rec.Files["SKILL.md"])
		assert.Equal(t, hashContent([]byte("b\n")), rec.Files["references/b.md"])

		// A second run skips it again rather than treating it as clean.
		_, res, err = applySkill(dir, skillFiles(v2), rec, true, "s2", false)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"SKILL.md": "modified"}, res.Skipped)
		assert.Empty(t, res.Written)
	})

	t.Run("force overwrites modified files", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		writeFiles(t, dir, map[string]string{"SKILL.md": "my edits\n"})
		rec, res, err := applySkill(dir, skillFiles(v2), recordFor(v1, "s1"), true, "s2", true)
		require.NoError(t, err)
		assert.Equal(t, v2, readAll(t, dir))
		assert.Empty(t, res.Skipped)
		assert.Equal(t, "s2", rec.SourceSHA)
	})

	t.Run("locally deleted file is not recreated", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		require.NoError(t, os.Remove(joinRel(dir, "SKILL.md")))
		_, res, err := applySkill(dir, skillFiles(v1), recordFor(v1, "s1"), true, "s1", false)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"SKILL.md": "deleted locally"}, res.Skipped)
		assert.NoFileExists(t, joinRel(dir, "SKILL.md"))
	})

	t.Run("untracked differing file is left alone, matching one adopted", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, map[string]string{"SKILL.md": "someone else's\n", "references/a.md": "a\n"})
		rec, res, err := applySkill(dir, skillFiles(v1), SkillRecord{}, false, "s1", false)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"SKILL.md": "unverified local copy"}, res.Skipped)
		assert.Equal(t, []string{"references/a.md"}, res.Adopted)
		_, tracked := rec.Files["SKILL.md"]
		assert.False(t, tracked, "a file we did not write is not claimed")
		assert.Equal(t, "", rec.SourceSHA)
	})

	t.Run("user-added file survives", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		writeFiles(t, dir, map[string]string{"notes/mine.md": "keep me\n"})
		_, _, err := applySkill(dir, skillFiles(v2), recordFor(v1, "s1"), true, "s2", false)
		require.NoError(t, err)
		assert.Equal(t, "keep me\n", readAll(t, dir)["notes/mine.md"])
	})
}

func TestRemoveSkillFiles(t *testing.T) {
	v1 := map[string]string{"SKILL.md": "v1\n", "references/a.md": "a\n"}

	t.Run("removes recorded files and empty dirs", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		removed, err := removeSkillFiles(dir, recordFor(v1, "s1"))
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"SKILL.md", "references/a.md"}, removed)
		assert.NoDirExists(t, dir)
	})

	t.Run("keeps user files and the directory holding them", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		writeFiles(t, dir, v1)
		writeFiles(t, dir, map[string]string{"references/mine.md": "keep\n"})
		_, err := removeSkillFiles(dir, recordFor(v1, "s1"))
		require.NoError(t, err)
		assert.FileExists(t, joinRel(dir, "references/mine.md"))
		assert.NoFileExists(t, joinRel(dir, "SKILL.md"))
		assert.NoFileExists(t, joinRel(dir, "references/a.md"))
	})

	t.Run("already missing files are fine", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "fly")
		removed, err := removeSkillFiles(dir, recordFor(v1, "s1"))
		require.NoError(t, err)
		assert.Empty(t, removed)
	})
}

func TestResolveTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".pi"), 0o755)) // not .pi/agent, so not detected

	t.Run("detects tools by parent dir and always includes universal", func(t *testing.T) {
		tools, err := resolveTools(home, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"universal", "claude-code", "opencode"}, toolSlugs(tools))

		targets := buildTargets(tools)
		assert.Equal(t, filepath.Join(home, ".agents", "skills"), targets[0].SkillsDir)
		assert.Equal(t, filepath.Join(home, ".claude", "skills"), targets[1].SkillsDir)
		assert.Equal(t, filepath.Join(home, ".config", "opencode", "skills"), targets[2].SkillsDir)
	})

	t.Run("explicit slugs ignore detection", func(t *testing.T) {
		tools, err := resolveTools(home, []string{"cursor", "claude-code", "cursor"})
		require.NoError(t, err)
		assert.Equal(t, []string{"cursor", "claude-code"}, toolSlugs(tools))
	})

	t.Run("unknown slug errors with the valid list", func(t *testing.T) {
		_, err := resolveTools(home, []string{"emacs"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown agent "emacs"`)
		assert.Contains(t, err.Error(), "claude-code")
	})

	t.Run("manifest keys map back to tool names", func(t *testing.T) {
		assert.Equal(t, "Claude Code", toolNameForKey(home, filepath.ToSlash(filepath.Join(home, ".claude", "skills"))))
		assert.Equal(t, "/elsewhere/skills", toolNameForKey(home, "/elsewhere/skills"))
	})
}

func TestManifestRoundTrip(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), ".fly")

	m, err := readManifest(configDir)
	require.NoError(t, err)
	assert.False(t, m.hasInstalledSkills())

	m.SourceSHA = "s1"
	m.setRecord("/home/x/.agents/skills", "fly", recordFor(map[string]string{"SKILL.md": "v1\n"}, "s1"))
	m.setRecord("/home/x/.agents/skills", "brand", recordFor(map[string]string{"SKILL.md": "b\n"}, "s2"))
	require.NoError(t, m.save(configDir))

	got, err := readManifest(configDir)
	require.NoError(t, err)
	assert.True(t, got.hasInstalledSkills())
	assert.Equal(t, "s1", got.SourceSHA)
	rec, ok := got.record("/home/x/.agents/skills", "fly")
	require.True(t, ok)
	assert.Equal(t, hashContent([]byte("v1\n")), rec.Files["SKILL.md"])

	sha, mixed := got.targetRevision("/home/x/.agents/skills")
	assert.True(t, mixed)
	assert.NotEmpty(t, sha)

	got.deleteRecord("/home/x/.agents/skills", "brand")
	sha, mixed = got.targetRevision("/home/x/.agents/skills")
	assert.False(t, mixed)
	assert.Equal(t, "s1", sha)

	got.deleteRecord("/home/x/.agents/skills", "fly")
	assert.False(t, got.hasInstalledSkills())

	t.Run("corrupt manifest is an error", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(manifestPath(dir), []byte("{nope"), 0o600))
		_, err := readManifest(dir)
		require.Error(t, err)
	})
}

func TestSourceURLs(t *testing.T) {
	src := source{repo: skillsRepo}
	assert.Equal(t, "https://api.github.com/repos/superfly/skills/commits/main", src.shaURL())
	assert.Equal(t, "https://github.com/superfly/skills/archive/refs/heads/main.tar.gz", src.tarballURL(""))
	assert.Equal(t, "https://github.com/superfly/skills/archive/0123456789abcdef0123456789abcdef01234567.tar.gz", src.tarballURL("0123456789abcdef0123456789abcdef01234567"))

	branch := source{repo: skillsRepo, ref: "alex/fly-skill-plugin"}
	assert.Equal(t, "https://api.github.com/repos/superfly/skills/commits/alex/fly-skill-plugin", branch.shaURL())
	assert.Equal(t, "https://github.com/superfly/skills/archive/refs/heads/alex/fly-skill-plugin.tar.gz", branch.tarballURL(""))
	assert.False(t, branch.pinned())

	pinned := source{repo: skillsRepo, ref: "0123456789abcdef0123456789abcdef01234567"}
	assert.True(t, pinned.pinned())

	local := source{repo: skillsRepo, tarball: "/tmp/skills.tar.gz"}
	assert.Equal(t, "/tmp/skills.tar.gz", local.tarballURL("whatever"))
}

func TestValidSkillName(t *testing.T) {
	for _, name := range []string{"fly", "brand", "fly-2", "a", "a1-b2-c3"} {
		assert.True(t, validSkillName(name), name)
	}
	for _, name := range []string{"", "Fly", "-fly", "fly-", "fly--x", "fly_x", "fly.x", "fly/x", `fly\x`, "fly x", ".", "..", "C:", strings.Repeat("a", 65)} {
		assert.False(t, validSkillName(name), name)
	}
	assert.True(t, validSkillName(strings.Repeat("a", 64)))
}

func TestValidRelPath(t *testing.T) {
	for _, rel := range []string{"SKILL.md", "references/a.md", "a/b/c.md", "a-b_c.d", "..a/b", "a/.b"} {
		assert.True(t, validRelPath(rel), rel)
	}
	for _, rel := range []string{
		"", ".", "..", "../x", "a/../x", "a/..", "./a", "a/./b", "/etc/passwd", "a//b", "a/",
		`..\..\x`, `a\..\..\x`, `a/..\b`, `a\b`, `C:\x`, `C:x`, `C:/x`, `\\server\share`, "a\x00b", "a:b",
	} {
		assert.False(t, validRelPath(rel), rel)
	}
}

func TestSafeJoin(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fly")

	got, err := safeJoin(dir, "references/a.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "references", "a.md"), got)

	for _, rel := range []string{"../x", `..\..\x`, `C:\x`, `\\server\share`, "/x", "a\x00b"} {
		_, err := safeJoin(dir, rel)
		require.Error(t, err, rel)
		assert.ErrorIs(t, err, errUnsafePath, rel)
	}
}

// A hand-edited manifest must never let remove or update delete or write
// outside the skill directory.
func TestManifestPathsCannotEscapeSkillDir(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "fly")
	victim := filepath.Join(root, "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("precious\n"), 0o644))
	writeFiles(t, skillDir, map[string]string{"SKILL.md": "v1\n"})

	for _, rel := range []string{"../../victim.txt", `..\..\victim.txt`, "/" + filepath.ToSlash(victim)} {
		rec := SkillRecord{Files: map[string]string{
			"SKILL.md": hashContent([]byte("v1\n")),
			rel:        hashContent([]byte("precious\n")),
		}}

		removed, err := removeSkillFiles(skillDir, rec)
		require.Error(t, err, rel)
		assert.ErrorIs(t, err, errUnsafePath)
		assert.Empty(t, removed, "nothing is deleted once an invalid path is found")
		assert.FileExists(t, victim)
		assert.FileExists(t, filepath.Join(skillDir, "SKILL.md"))

		// Upgrading treats the bogus entry as a dropped file and must refuse
		// rather than remove it.
		_, _, err = applySkill(skillDir, skillFiles(map[string]string{"SKILL.md": "v2\n"}), rec, true, "s2", false)
		require.Error(t, err, rel)
		assert.ErrorIs(t, err, errUnsafePath)
		assert.FileExists(t, victim)

		// Status reports it as modified instead of reading outside the dir.
		assert.Contains(t, modifiedFiles(skillDir, rec), rel)
	}

	// An incoming file with a bad path is refused before anything is written.
	_, _, err := applySkill(skillDir, []SkillFile{{Rel: `..\..\victim.txt`, Contents: []byte("owned\n")}}, SkillRecord{}, false, "s1", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUnsafePath)
	body, _ := os.ReadFile(victim)
	assert.Equal(t, "precious\n", string(body))
}
