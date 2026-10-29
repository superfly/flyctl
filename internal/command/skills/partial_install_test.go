package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartialInstallRecordsCompletedFilesAndContinues(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "fresh"
		if existing {
			name = "update"
		}
		t.Run(name, func(t *testing.T) {
			ctx, e, flags, _ := skillsTestContext(t)
			first := Target{ToolName: "First", SkillsDir: filepath.Join(e.home, "first", "skills")}
			last := Target{ToolName: "Last", SkillsDir: filepath.Join(e.home, "last", "skills")}
			archive, err := flags.GetString("tarball")
			require.NoError(t, err)
			writeArchive := func(body string, withReference bool) {
				entries := []tarEntry{
					{name: "skills-main/plugins/fly/skills/fly/SKILL.md", body: body},
					{name: "skills-main/plugins/fly/skills/fly/z-last.md", body: body},
				}
				if withReference {
					entries = append(entries, tarEntry{name: "skills-main/plugins/fly/skills/fly/references/new.md", body: body})
				}
				require.NoError(t, os.WriteFile(archive, makeTarball(t, entries), 0o644))
			}
			if existing {
				writeArchive("old", false)
				require.NoError(t, syncSkills(ctx, e, []Target{first, last}, "installed", false, true))
			}
			blocked := filepath.Join(first.SkillsDir, "fly", "references")
			require.NoError(t, os.MkdirAll(filepath.Dir(blocked), 0o755))
			require.NoError(t, os.WriteFile(blocked, []byte("user file"), 0o644))
			writeArchive("new", true)
			require.NoError(t, flags.Set("ref", testNewSHA))

			results, err := InstallInto(ctx, []Target{first, last}, "installed", false, true)
			require.NoError(t, err, "only the first target failed")
			require.Error(t, results[first.Key()].Err)
			require.NoError(t, results[last.Key()].Err)
			m, err := readManifest(e.configDir)
			require.NoError(t, err)
			partial, ok := m.record(first.Key(), "fly")
			require.True(t, ok)
			require.Equal(t, hashContent([]byte("new")), partial.Files["SKILL.md"])
			require.NotContains(t, partial.Files, "references/new.md")
			if existing {
				require.Equal(t, testOldSHA, partial.SourceSHA)
				require.Equal(t, testOldSHA, m.SourceSHA)
				require.Equal(t, hashContent([]byte("old")), partial.Files["z-last.md"])
			} else {
				require.Empty(t, partial.SourceSHA)
				require.Empty(t, m.SourceSHA)
				require.Len(t, partial.Files, 1)
			}
			complete, ok := m.record(last.Key(), "fly")
			require.True(t, ok)
			require.Equal(t, testNewSHA, complete.SourceSHA)
			require.Len(t, complete.Files, 3)
			require.Equal(t, testNewSHA, m.LatestSHA)
			require.FileExists(t, filepath.Join(last.SkillsDir, "fly", "references", "new.md"))

			// The ordinary skills commands use the same saved receipts and
			// report failure too; a repaired directory can then finish normally.
			require.Error(t, syncSkills(ctx, e, []Target{first, last}, "updated", false, true))
			require.NoError(t, os.Remove(blocked))
			require.NoError(t, runUpdate(ctx))
			m, err = readManifest(e.configDir)
			require.NoError(t, err)
			require.Equal(t, testNewSHA, m.SourceSHA)
			complete, ok = m.record(first.Key(), "fly")
			require.True(t, ok)
			require.Equal(t, testNewSHA, complete.SourceSHA)
			require.Len(t, complete.Files, 3)
		})
	}
}
