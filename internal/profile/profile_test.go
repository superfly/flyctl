package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setup points the profile store at a scratch directory and clears every
// environment variable that participates in resolution, so a test only sees
// the inputs it sets itself.
func setup(t *testing.T) string {
	t.Helper()

	home := t.TempDir()

	t.Setenv(HomeEnvKey, home)
	t.Setenv(ConfigDirEnvKey, "")
	t.Setenv(EnvKey, "")

	// Setenv to "" still leaves the variable set, which Resolve treats as
	// unset only because it trims and checks for empty. Unset them outright so
	// the test exercises the real code path.
	require.NoError(t, os.Unsetenv(ConfigDirEnvKey))
	require.NoError(t, os.Unsetenv(EnvKey))

	return home
}

func mustCreate(t *testing.T, name string) string {
	t.Helper()

	dir, err := Create(name)
	require.NoError(t, err)

	return dir
}

func TestValidateName(t *testing.T) {
	valid := []string{"work", "client-a", "acme_prod", "a", "a.b", "A1"}
	for _, name := range valid {
		assert.NoError(t, ValidateName(name), "expected %q to be valid", name)
	}

	// Names become path components, so anything that could escape the store or
	// collide with the store's own files must be refused.
	invalid := []string{"", ".", "..", "../escape", "a/b", "a\\b", "-leading", ".hidden", "with space"}
	for _, name := range invalid {
		assert.Error(t, ValidateName(name), "expected %q to be invalid", name)
	}
}

func TestDir(t *testing.T) {
	home := setup(t)

	dir, err := Dir(Default)
	require.NoError(t, err)
	assert.Equal(t, home, dir, "the default profile is the config directory itself")

	dir, err = Dir("work")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "profiles", "work"), dir)

	_, err = Dir("../escape")
	assert.Error(t, err)
}

func TestResolveDefaultsToDefaultProfile(t *testing.T) {
	home := setup(t)

	res, err := Resolve(ResolveOptions{})
	require.NoError(t, err)

	assert.Equal(t, Default, res.Name)
	assert.Equal(t, home, res.Dir)
	assert.Equal(t, SourceDefault, res.Source)
}

func TestResolveConfigDirEnvWins(t *testing.T) {
	setup(t)
	mustCreate(t, "work")
	require.NoError(t, SetActive("work"))

	pinned := t.TempDir()
	t.Setenv(ConfigDirEnvKey, pinned)
	t.Setenv(EnvKey, "work")

	res, err := Resolve(ResolveOptions{Flag: "work"})
	require.NoError(t, err)

	assert.Equal(t, pinned, res.Dir)
	assert.Equal(t, SourceConfigDirEnv, res.Source)
	assert.Empty(t, res.Name, "pinning a directory bypasses profiles entirely")
}

func TestResolvePrecedence(t *testing.T) {
	home := setup(t)

	for _, name := range []string{"flagged", "envd", "linked", "active"} {
		mustCreate(t, name)
	}
	require.NoError(t, SetActive("active"))

	project := t.TempDir()
	_, err := WriteProjectFile(project, "linked")
	require.NoError(t, err)

	t.Run("flag beats everything else", func(t *testing.T) {
		t.Setenv(EnvKey, "envd")

		res, err := Resolve(ResolveOptions{Flag: "flagged", WorkingDir: project})
		require.NoError(t, err)

		assert.Equal(t, "flagged", res.Name)
		assert.Equal(t, SourceFlag, res.Source)
	})

	t.Run("env beats the project file", func(t *testing.T) {
		t.Setenv(EnvKey, "envd")

		res, err := Resolve(ResolveOptions{WorkingDir: project})
		require.NoError(t, err)

		assert.Equal(t, "envd", res.Name)
		assert.Equal(t, SourceEnv, res.Source)
	})

	t.Run("project file beats the active profile", func(t *testing.T) {
		res, err := Resolve(ResolveOptions{WorkingDir: project})
		require.NoError(t, err)

		assert.Equal(t, "linked", res.Name)
		assert.Equal(t, SourceProjectFile, res.Source)
		assert.Equal(t, filepath.Join(project, ProjectFileName), res.Detail)
	})

	t.Run("active profile is the fallback", func(t *testing.T) {
		res, err := Resolve(ResolveOptions{WorkingDir: t.TempDir()})
		require.NoError(t, err)

		assert.Equal(t, "active", res.Name)
		assert.Equal(t, SourceActive, res.Source)
		assert.Equal(t, filepath.Join(home, "profiles", "active"), res.Dir)
	})
}

// A profile that has been deleted, or was never created, must stop the command
// rather than quietly falling back: silently reaching a different Fly.io
// account is the failure this whole feature exists to prevent.
func TestResolveMissingProfileIsAnError(t *testing.T) {
	setup(t)

	_, err := Resolve(ResolveOptions{Flag: "ghost"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	assert.Contains(t, err.Error(), "fly profile add ghost")

	t.Setenv(EnvKey, "ghost")
	_, err = Resolve(ResolveOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), string(SourceEnv))
}

func TestResolveNamesTheProjectFileInErrors(t *testing.T) {
	setup(t)

	project := t.TempDir()
	_, err := WriteProjectFile(project, "ghost")
	require.NoError(t, err)

	_, err = Resolve(ResolveOptions{WorkingDir: project})
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(project, ProjectFileName))
}

func TestFindProjectFileWalksUp(t *testing.T) {
	setup(t)

	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nested, 0o700))

	path, err := WriteProjectFile(root, "work")
	require.NoError(t, err)

	foundPath, name, err := FindProjectFile(nested)
	require.NoError(t, err)
	assert.Equal(t, path, foundPath)
	assert.Equal(t, "work", name)

	// The nearest file wins, so a deeper binding overrides a shallower one.
	deeper, err := WriteProjectFile(filepath.Join(root, "a"), "other")
	require.NoError(t, err)

	foundPath, name, err = FindProjectFile(nested)
	require.NoError(t, err)
	assert.Equal(t, deeper, foundPath)
	assert.Equal(t, "other", name)
}

func TestFindProjectFileIgnoresTrailingContent(t *testing.T) {
	setup(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ProjectFileName),
		[]byte("work\n# the billing account for this client\n"),
		0o644,
	))

	_, name, err := FindProjectFile(dir)
	require.NoError(t, err)
	assert.Equal(t, "work", name)
}

func TestListIsDefaultFirstThenSorted(t *testing.T) {
	home := setup(t)

	for _, name := range []string{"zeta", "alpha", "mid"} {
		mustCreate(t, name)
	}

	profiles, err := List()
	require.NoError(t, err)

	var names []string
	for _, p := range profiles {
		names = append(names, p.Name)
	}

	assert.Equal(t, []string{Default, "alpha", "mid", "zeta"}, names)
	assert.Equal(t, home, profiles[0].Dir)
}

func TestActiveRoundTrip(t *testing.T) {
	setup(t)
	mustCreate(t, "work")

	active, err := Active()
	require.NoError(t, err)
	assert.Equal(t, Default, active)

	require.NoError(t, SetActive("work"))

	active, err = Active()
	require.NoError(t, err)
	assert.Equal(t, "work", active)

	// Selecting the default profile clears the pointer rather than writing it.
	require.NoError(t, SetActive(Default))

	active, err = Active()
	require.NoError(t, err)
	assert.Equal(t, Default, active)
}

// Removing the active profile must retire the pointer with it, or every later
// command fails on a dangling reference.
func TestRemoveClearsActivePointer(t *testing.T) {
	setup(t)

	dir := mustCreate(t, "work")
	require.NoError(t, SetActive("work"))
	require.NoError(t, Remove("work"))

	assert.NoDirExists(t, dir)

	active, err := Active()
	require.NoError(t, err)
	assert.Equal(t, Default, active)
}

func TestRemoveAndRenameRefuseTheDefaultProfile(t *testing.T) {
	setup(t)
	mustCreate(t, "work")

	assert.Error(t, Remove(Default))
	assert.Error(t, Rename(Default, "work2"))
	assert.Error(t, Rename("work", Default))
}

func TestRenameCarriesTheActivePointer(t *testing.T) {
	setup(t)

	mustCreate(t, "old")
	require.NoError(t, SetActive("old"))
	require.NoError(t, Rename("old", "new"))

	active, err := Active()
	require.NoError(t, err)
	assert.Equal(t, "new", active)

	exists, err := Exists("old")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCreateRefusesDuplicatesAndDefault(t *testing.T) {
	setup(t)

	mustCreate(t, "work")

	_, err := Create("work")
	assert.Error(t, err)

	_, err = Create(Default)
	assert.Error(t, err)
}

func TestMetadataRoundTrip(t *testing.T) {
	setup(t)
	mustCreate(t, "work")

	require.NoError(t, WriteMetadata("work", Metadata{Email: "a@example.com"}))

	md, err := ReadMetadata("work")
	require.NoError(t, err)
	assert.Equal(t, "a@example.com", md.Email)

	// Metadata is a display convenience, so a profile without any reads as a
	// zero value rather than an error.
	mustCreate(t, "bare")
	md, err = ReadMetadata("bare")
	require.NoError(t, err)
	assert.Empty(t, md.Email)
}

// The profile management commands run through Fallback so a dangling
// reference cannot lock the user out of the commands that repair it.
func TestFallbackPointsAtDefaultAndKeepsTheCause(t *testing.T) {
	home := setup(t)

	_, cause := Resolve(ResolveOptions{Flag: "ghost"})
	require.Error(t, cause)

	res, err := Fallback(cause)
	require.NoError(t, err)

	assert.Equal(t, Default, res.Name)
	assert.Equal(t, home, res.Dir)
	assert.Equal(t, cause, res.Err)
}

func TestFlagFromArgs(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"deploy", "--profile", "work"}, "work"},
		{[]string{"deploy", "--profile=work"}, "work"},
		{[]string{"--profile", "work", "deploy"}, "work"},
		{[]string{"deploy"}, ""},
		{[]string{"deploy", "--profile"}, ""},
		{[]string{"ssh", "console", "-C", "run --profile prod"}, ""},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.want, FlagFromArgs(tc.args), "args: %v", tc.args)
	}
}
