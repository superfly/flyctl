package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

const (
	testOldSHA = "1111111111111111111111111111111111111111"
	testNewSHA = "2222222222222222222222222222222222222222"
)

func skillsTestContext(t *testing.T) (context.Context, env, *pflag.FlagSet, *bytes.Buffer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(envRef, "")
	t.Setenv(envTarball, "")
	e := env{home: home, configDir: filepath.Join(home, ".fly")}
	var out bytes.Buffer
	ctx := iostreams.NewContext(context.Background(), &iostreams.IOStreams{Out: &out, ErrOut: &out})
	ctx = state.WithConfigDirectory(ctx, e.configDir)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: true})
	flags := pflag.NewFlagSet("skills-test", pflag.ContinueOnError)
	flags.String("ref", "", "")
	flags.String("tarball", filepath.Join(home, "skills.tar.gz"), "")
	flags.StringSlice("agent", nil, "")
	flags.Bool("force", false, "")
	flags.Bool("offline", true, "")
	flags.Bool("yes", false, "")
	require.NoError(t, flags.Set("ref", testOldSHA))

	return flag.NewContext(ctx, flags), e, flags, &out
}

// unsetFlag puts a flag back to how cobra leaves one the user did not pass.
func unsetFlag(t *testing.T, flags *pflag.FlagSet, name string) {
	t.Helper()
	f := flags.Lookup(name)
	require.NotNil(t, f)
	require.NoError(t, f.Value.Set(f.DefValue))
	f.Changed = false
}

func writeSkillsArchive(t *testing.T, flags *pflag.FlagSet, names ...string) {
	t.Helper()
	var entries []tarEntry
	for _, name := range names {
		entries = append(entries, tarEntry{name: "skills-main/plugins/fly/skills/" + name + "/SKILL.md", body: name})
	}
	location, err := flags.GetString("tarball")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(location, makeTarball(t, entries), 0o644))
}

func linkSkill(t *testing.T, source, destination string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0o755))
	err := os.Symlink(source, destination)
	if err != nil && runtime.GOOS == "windows" {
		t.Skipf("creating a symlink requires Windows developer mode or privileges: %v", err)
	}
	require.NoError(t, err)
}

func TestRemoveLinkedSkillPreservesOtherTarget(t *testing.T) {
	ctx, e, flags, _ := skillsTestContext(t)
	universal := filepath.Join(e.home, ".agents", "skills")
	claude := filepath.Join(e.home, ".claude", "skills")
	writeFiles(t, joinRel(universal, "fly"), map[string]string{"SKILL.md": "fly"})
	linkSkill(t, joinRel(universal, "fly"), joinRel(claude, "fly"))
	writeSkillsArchive(t, flags, "fly")
	require.NoError(t, syncSkills(ctx, e, []Target{{SkillsDir: universal}, {SkillsDir: claude}}, "installed", false))
	// A shared local edit and a user-added file must survive removing the link.
	writeFiles(t, joinRel(universal, "fly"), map[string]string{"SKILL.md": "local edit", "notes.md": "keep"})
	require.NoError(t, flags.Set("agent", "claude-code"))
	require.NoError(t, runRemove(ctx))

	body, err := os.ReadFile(joinRel(universal, "fly/SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "local edit", string(body))
	assert.FileExists(t, joinRel(universal, "fly/notes.md"))
	_, err = os.Lstat(joinRel(claude, "fly"))
	assert.True(t, os.IsNotExist(err), "the agent link should be removed")
	m, err := readManifest(e.configDir)
	require.NoError(t, err)
	_, tracked := m.record(filepath.ToSlash(universal), "fly")
	assert.True(t, tracked)
	_, tracked = m.record(filepath.ToSlash(claude), "fly")
	assert.False(t, tracked)
}

func TestSyncSkillsRetiresDroppedSkill(t *testing.T) {
	for _, tc := range []struct {
		name     string
		modified bool
		userFile bool
	}{
		{name: "clean"},
		{name: "user-added file", userFile: true},
		{name: "modified then forced", modified: true, userFile: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, e, flags, out := skillsTestContext(t)
			target := Target{SkillsDir: filepath.Join(e.home, ".agents", "skills")}
			writeSkillsArchive(t, flags, "brand", "fly")
			require.NoError(t, syncSkills(ctx, e, []Target{target}, "installed", false))
			brandDir := joinRel(target.SkillsDir, "brand")
			if tc.modified {
				writeFiles(t, brandDir, map[string]string{"SKILL.md": "local edit"})
			}
			if tc.userFile {
				writeFiles(t, brandDir, map[string]string{"notes.md": "keep"})
			}

			writeSkillsArchive(t, flags, "fly")
			require.NoError(t, flags.Set("ref", testNewSHA))
			require.NoError(t, runUpdate(ctx))
			m, err := readManifest(e.configDir)
			require.NoError(t, err)
			rec, tracked := m.record(target.Key(), "brand")
			assert.Equal(t, tc.modified, tracked)
			if tc.modified {
				assert.Equal(t, testOldSHA, rec.SourceSHA)
				assert.Contains(t, out.String(), "kept your local copy of brand")
				body, err := os.ReadFile(joinRel(brandDir, "SKILL.md"))
				require.NoError(t, err)
				assert.Equal(t, "local edit", string(body))
				// Repeating the update must continue preserving the local edit.
				require.NoError(t, runUpdate(ctx))
				require.NoError(t, flags.Set("force", "true"))
				require.NoError(t, runUpdate(ctx))
			}

			assert.NoFileExists(t, joinRel(brandDir, "SKILL.md"))
			if tc.userFile {
				assert.FileExists(t, joinRel(brandDir, "notes.md"))
			} else {
				assert.NoDirExists(t, brandDir)
			}
			assert.FileExists(t, joinRel(target.SkillsDir, "fly/SKILL.md"))
			m, err = readManifest(e.configDir)
			require.NoError(t, err)
			_, tracked = m.record(target.Key(), "brand")
			assert.False(t, tracked)
			out.Reset()
			require.NoError(t, runStatus(ctx))
			var report statusReport
			require.NoError(t, json.Unmarshal(out.Bytes(), &report))
			assert.False(t, report.UpdateAvailable)
		})
	}
}

func TestSyncSkillsRetiresLinkedSkillWithoutDeletingSharedFiles(t *testing.T) {
	ctx, e, flags, _ := skillsTestContext(t)
	target := Target{SkillsDir: filepath.Join(e.home, ".claude", "skills")}
	shared := filepath.Join(e.home, "shared-brand")
	writeFiles(t, shared, map[string]string{"SKILL.md": "brand"})
	linkSkill(t, shared, joinRel(target.SkillsDir, "brand"))
	writeSkillsArchive(t, flags, "brand", "fly")
	require.NoError(t, syncSkills(ctx, e, []Target{target}, "installed", false))
	writeSkillsArchive(t, flags, "fly")
	require.NoError(t, flags.Set("ref", testNewSHA))
	require.NoError(t, runUpdate(ctx))

	assert.FileExists(t, joinRel(shared, "SKILL.md"))
	_, err := os.Lstat(joinRel(target.SkillsDir, "brand"))
	assert.True(t, os.IsNotExist(err))
	m, err := readManifest(e.configDir)
	require.NoError(t, err)
	_, tracked := m.record(target.Key(), "brand")
	assert.False(t, tracked)
}

func TestStatusReportsPartialAdoptionAsMixed(t *testing.T) {
	for _, modified := range []string{"brand", "fly"} {
		t.Run(modified, func(t *testing.T) {
			ctx, e, flags, out := skillsTestContext(t)
			target := Target{SkillsDir: filepath.Join(e.home, ".agents", "skills")}
			writeFiles(t, joinRel(target.SkillsDir, modified), map[string]string{"SKILL.md": "my existing skill"})
			writeSkillsArchive(t, flags, "brand", "fly")
			require.NoError(t, syncSkills(ctx, e, []Target{target}, "installed", false))
			out.Reset()
			require.NoError(t, runStatus(ctx))

			var report statusReport
			require.NoError(t, json.Unmarshal(out.Bytes(), &report))
			require.Len(t, report.Targets, 1)
			assert.True(t, report.Targets[0].Mixed)
			assert.True(t, report.UpdateAvailable)
		})
	}
}
