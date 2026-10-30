package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestRoundTripSourceRef(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), ".fly")

	m, err := readManifest(configDir)
	require.NoError(t, err)
	assert.Empty(t, m.SourceRef)

	m.SourceRef = "v1.2.0"
	m.setRecord("/home/x/.agents/skills", "fly", recordFor(map[string]string{"SKILL.md": "v1\n"}, "s1"))
	require.NoError(t, m.save(configDir))

	got, err := readManifest(configDir)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.0", got.SourceRef)
	data, err := os.ReadFile(manifestPath(configDir))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"source_ref": "v1.2.0"`)

	// Clearing the pin drops the key rather than writing an empty string.
	got.SourceRef = ""
	require.NoError(t, got.save(configDir))
	data, err = os.ReadFile(manifestPath(configDir))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "source_ref")
}

func TestResolveRefPrecedence(t *testing.T) {
	ctx, _, flags, _ := skillsTestContext(t)
	pinned := &Manifest{SourceRef: "v1"}

	// The harness passes --ref, which wins over everything.
	t.Setenv(envRef, "env-branch")
	assert.Equal(t, testOldSHA, resolveRef(ctx, pinned))

	unsetFlag(t, flags, "ref")
	assert.Equal(t, "env-branch", resolveRef(ctx, pinned))

	t.Setenv(envRef, "")
	assert.Equal(t, "v1", resolveRef(ctx, pinned))
	assert.Equal(t, "", resolveRef(ctx, &Manifest{}))
	assert.Equal(t, "", resolveRef(ctx, nil))

	// Naming main at any level means "track main", not a pin called main.
	require.NoError(t, flags.Set("ref", "main"))
	assert.Equal(t, "", resolveRef(ctx, pinned))
	unsetFlag(t, flags, "ref")
	t.Setenv(envRef, "main")
	assert.Equal(t, "", resolveRef(ctx, pinned))
	t.Setenv(envRef, "")
	assert.Equal(t, "", resolveRef(ctx, &Manifest{SourceRef: "main"}))
}

func TestUpdateHonoursStoredPin(t *testing.T) {
	ctx, e, flags, out := skillsTestContext(t)
	target := Target{SkillsDir: filepath.Join(e.home, ".agents", "skills")}
	writeSkillsArchive(t, flags, "fly")

	// The harness installs with --ref <sha>, which records the pin.
	require.NoError(t, syncSkills(ctx, e, []Target{target}, "installed", false, false))
	m, err := readManifest(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, testOldSHA, m.SourceRef)
	assert.Equal(t, testOldSHA, m.SourceSHA)
	assert.Contains(t, out.String(), "Pinned to "+testOldSHA)
	assert.Contains(t, out.String(), "--ref main")

	// An update that does not mention --ref stays on the pin instead of
	// drifting back to main.
	unsetFlag(t, flags, "ref")
	out.Reset()
	require.NoError(t, runUpdate(ctx))
	m, err = readManifest(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, testOldSHA, m.SourceRef)
	assert.Equal(t, testOldSHA, m.SourceSHA)
	assert.Contains(t, out.String(), "Pinned to "+testOldSHA)
}

func TestUpdateRefMainClearsPin(t *testing.T) {
	ctx, e, flags, out := skillsTestContext(t)
	target := Target{SkillsDir: filepath.Join(e.home, ".agents", "skills")}
	writeSkillsArchive(t, flags, "fly")
	require.NoError(t, syncSkills(ctx, e, []Target{target}, "installed", false, false))

	require.NoError(t, flags.Set("ref", "main"))
	out.Reset()
	require.NoError(t, runUpdate(ctx))
	m, err := readManifest(e.configDir)
	require.NoError(t, err)
	assert.Empty(t, m.SourceRef)
	assert.NotContains(t, out.String(), "Pinned to")
	data, err := os.ReadFile(manifestPath(e.configDir))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "source_ref")

	// Once unpinned, a plain update keeps tracking main.
	unsetFlag(t, flags, "ref")
	require.NoError(t, runUpdate(ctx))
	m, err = readManifest(e.configDir)
	require.NoError(t, err)
	assert.Empty(t, m.SourceRef)
}

func TestUpdateRefPrecedence(t *testing.T) {
	ctx, e, flags, out := skillsTestContext(t)
	target := Target{SkillsDir: filepath.Join(e.home, ".agents", "skills")}
	writeSkillsArchive(t, flags, "fly")
	require.NoError(t, syncSkills(ctx, e, []Target{target}, "installed", false, false))

	// The environment beats the manifest's pin.
	unsetFlag(t, flags, "ref")
	t.Setenv(envRef, testNewSHA)
	out.Reset()
	require.NoError(t, runUpdate(ctx))
	m, err := readManifest(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, testNewSHA, m.SourceRef)
	assert.Equal(t, testNewSHA, m.SourceSHA)
	assert.Contains(t, out.String(), "Pinned to "+testNewSHA)

	// An explicit --ref beats the environment.
	require.NoError(t, flags.Set("ref", testOldSHA))
	require.NoError(t, runUpdate(ctx))
	m, err = readManifest(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, testOldSHA, m.SourceRef)
	assert.Equal(t, testOldSHA, m.SourceSHA)
}

func TestStatusComparesAgainstPinWithoutWritingIt(t *testing.T) {
	ctx, e, flags, out := skillsTestContext(t)
	target := Target{SkillsDir: filepath.Join(e.home, ".agents", "skills")}
	writeSkillsArchive(t, flags, "fly")
	require.NoError(t, syncSkills(ctx, e, []Target{target}, "installed", false, false))

	status := func() statusReport {
		t.Helper()
		out.Reset()
		require.NoError(t, runStatus(ctx))
		var report statusReport
		require.NoError(t, json.Unmarshal(out.Bytes(), &report))

		return report
	}
	pin := func() string {
		t.Helper()
		m, err := readManifest(e.configDir)
		require.NoError(t, err)

		return m.SourceRef
	}

	// Without --ref, status compares against the pinned commit, so a pinned
	// install is up to date rather than forever behind main.
	unsetFlag(t, flags, "ref")
	report := status()
	assert.Equal(t, testOldSHA, report.SourceRef)
	assert.Equal(t, testOldSHA, report.CheckedRef)
	assert.Equal(t, testOldSHA, report.LatestSHA)
	assert.False(t, report.UpdateAvailable)
	assert.Contains(t, out.String(), `"source_ref"`)

	// --ref on status compares against another ref but leaves the pin alone.
	require.NoError(t, flags.Set("ref", testNewSHA))
	report = status()
	assert.Equal(t, testOldSHA, report.SourceRef)
	assert.Equal(t, testNewSHA, report.CheckedRef)
	assert.Equal(t, testNewSHA, report.LatestSHA)
	assert.True(t, report.UpdateAvailable)
	assert.Equal(t, testOldSHA, pin())

	require.NoError(t, flags.Set("ref", "main"))
	report = status()
	assert.Equal(t, "main", report.CheckedRef)
	assert.Equal(t, testOldSHA, pin())

	// Nor does the environment change what is recorded.
	unsetFlag(t, flags, "ref")
	t.Setenv(envRef, testNewSHA)
	report = status()
	assert.Equal(t, testNewSHA, report.CheckedRef)
	assert.Equal(t, testOldSHA, pin())
}

func TestInstallWithAgentWritesOnlyThoseAgents(t *testing.T) {
	ctx, e, flags, _ := skillsTestContext(t)
	require.NoError(t, os.MkdirAll(filepath.Join(e.home, ".claude"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(e.home, ".cursor"), 0o755))
	writeSkillsArchive(t, flags, "fly")
	require.NoError(t, flags.Set("yes", "true"))
	require.NoError(t, flags.Set("agent", "claude-code"))

	require.NoError(t, runInstall(ctx))

	claude := filepath.Join(e.home, ".claude", "skills")
	assert.FileExists(t, joinRel(claude, "fly/SKILL.md"))
	// Naming an agent replaces detection: neither the universal directory
	// nor the other detected agent is written.
	assert.NoDirExists(t, filepath.Join(e.home, ".agents"))
	assert.NoDirExists(t, filepath.Join(e.home, ".cursor", "skills"))
	m, err := readManifest(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.ToSlash(claude)}, sortedKeys(m.Targets))
}
