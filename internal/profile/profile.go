// Package profile implements named, fully isolated flyctl credential
// profiles, so a single machine can drive multiple Fly.io accounts without
// logging in and out.
//
// A profile is nothing more than a flyctl config directory. The default
// profile is the legacy `~/.fly` directory itself, which keeps an existing
// installation working untouched; named profiles live beside it under
// `~/.fly/profiles/<name>`. Because a profile is a whole config directory and
// not just a token, each one carries its own access token, metrics token,
// WireGuard peer state and agent socket.
package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// Default denotes the name of the profile backed by the legacy config
	// directory.
	Default = "default"

	// FlagName denotes the name of the global profile flag.
	FlagName = "profile"

	// EnvKey denotes the environment variable that selects a profile by name.
	EnvKey = "FLY_PROFILE"

	// HomeEnvKey denotes the environment variable that overrides where
	// profiles are stored.
	HomeEnvKey = "FLY_PROFILE_HOME"

	// ConfigDirEnvKey denotes the environment variable that pins the config
	// directory outright, bypassing profile resolution.
	ConfigDirEnvKey = "FLY_CONFIG_DIR"

	// ProjectFileName denotes the name of the file that binds a directory
	// tree to a profile.
	ProjectFileName = ".fly-profile"

	// MetadataFileName denotes the name of the file holding a profile's
	// router-managed metadata.
	MetadataFileName = "profile.yml"

	profilesDirName = "profiles"
	activeFileName  = "active_profile"

	dirPerm  = 0o700
	filePerm = 0o600
)

// Source describes how a profile came to be selected.
type Source string

const (
	SourceConfigDirEnv Source = "FLY_CONFIG_DIR"
	SourceFlag         Source = "--profile"
	SourceEnv          Source = "FLY_PROFILE"
	SourceProjectFile  Source = ".fly-profile"
	SourceActive       Source = "active profile"
	SourceDefault      Source = "default"
)

// Resolution is the outcome of resolving which config directory to use.
type Resolution struct {
	// Name is the resolved profile name. It is empty when ConfigDirEnvKey
	// pinned the directory, since that bypasses profiles entirely.
	Name string

	// Dir is the config directory flyctl should read and write.
	Dir string

	// Source records which rule selected the profile.
	Source Source

	// Detail carries extra context about the source, such as the path of the
	// .fly-profile file that matched. It may be empty.
	Detail string

	// Err records why resolution failed, on the tolerated paths that fall back
	// to the default profile rather than refusing to run. It is nil whenever
	// the profile was selected normally.
	Err error
}

// Metadata is the router-managed bookkeeping stored alongside a profile's
// flyctl config. None of it is authoritative; it exists so `fly profile list`
// can name the account behind a profile without a round trip per profile.
type Metadata struct {
	Email      string    `yaml:"email,omitempty"`
	CreatedAt  time.Time `yaml:"created_at,omitempty"`
	VerifiedAt time.Time `yaml:"verified_at,omitempty"`
}

// Profile describes a single stored profile.
type Profile struct {
	Name     string
	Dir      string
	Metadata Metadata
}

// ErrNotExist is returned when a named profile has no directory on disk.
var ErrNotExist = errors.New("profile does not exist")

// nameRE bounds profile names to what is safe as a single path component.
// Names become directory names, so anything that could escape the profile
// store is rejected outright.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidateName reports whether name is usable as a profile name.
func ValidateName(name string) error {
	switch {
	case name == "":
		return errors.New("profile name is empty")
	case name == "." || name == "..":
		return fmt.Errorf("%q is not a valid profile name", name)
	case !nameRE.MatchString(name):
		return fmt.Errorf(
			"%q is not a valid profile name: use 1-64 characters of letters, digits, dot, dash or underscore, starting with a letter or digit",
			name,
		)
	default:
		return nil
	}
}

// Home returns the directory the profile store lives in. It is the legacy
// flyctl config directory unless HomeEnvKey overrides it.
//
// Home deliberately ignores ConfigDirEnvKey: that variable pins a single
// config directory, and the profile store must stay put regardless.
func Home() (string, error) {
	if v := strings.TrimSpace(os.Getenv(HomeEnvKey)); v != "" {
		return v, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed determining home directory: %w", err)
	}

	return filepath.Join(home, ".fly"), nil
}

// Dir returns the config directory backing the named profile. It does not
// check whether that directory exists.
func Dir(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}

	home, err := Home()
	if err != nil {
		return "", err
	}

	if name == Default {
		return home, nil
	}

	return filepath.Join(home, profilesDirName, name), nil
}

// Exists reports whether the named profile is present on disk. The default
// profile always exists, since it is the config directory itself.
func Exists(name string) (bool, error) {
	if name == Default {
		return true, nil
	}

	dir, err := Dir(name)
	if err != nil {
		return false, err
	}

	switch fi, err := os.Stat(dir); {
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	case err != nil:
		return false, err
	default:
		return fi.IsDir(), nil
	}
}

// List returns every stored profile, default first and the rest sorted by
// name.
func List() ([]Profile, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}

	out := []Profile{{Name: Default, Dir: home, Metadata: readMetadata(home)}}

	entries, err := os.ReadDir(filepath.Join(home, profilesDirName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	var named []Profile
	for _, entry := range entries {
		if !entry.IsDir() || ValidateName(entry.Name()) != nil {
			continue
		}

		dir := filepath.Join(home, profilesDirName, entry.Name())
		named = append(named, Profile{
			Name:     entry.Name(),
			Dir:      dir,
			Metadata: readMetadata(dir),
		})
	}

	sort.Slice(named, func(i, j int) bool { return named[i].Name < named[j].Name })

	return append(out, named...), nil
}

// Create makes the directory backing the named profile and returns its path.
// It reports an error if the profile already exists.
func Create(name string) (string, error) {
	if name == Default {
		return "", fmt.Errorf("the %q profile always exists and cannot be created", Default)
	}

	exists, err := Exists(name)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("profile %q already exists", name)
	}

	dir, err := Dir(name)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return "", fmt.Errorf("failed creating profile directory: %w", err)
	}

	return dir, nil
}

// Remove deletes the named profile and everything in it. If the profile was
// active, the active pointer falls back to the default profile.
func Remove(name string) error {
	if name == Default {
		return fmt.Errorf("the %q profile cannot be removed; use `fly auth logout` to clear its credentials", Default)
	}

	exists, err := Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrNotExist, name)
	}

	dir, err := Dir(name)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed removing profile directory: %w", err)
	}

	// Leaving the active pointer dangling would break every subsequent
	// command, so retire it along with the profile.
	if active, err := Active(); err == nil && active == name {
		return SetActive(Default)
	}

	return nil
}

// Rename moves the profile stored under oldName to newName, carrying the
// active pointer across if it referred to the renamed profile.
func Rename(oldName, newName string) error {
	if oldName == Default || newName == Default {
		return fmt.Errorf("the %q profile cannot be renamed", Default)
	}

	exists, err := Exists(oldName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrNotExist, oldName)
	}

	if exists, err := Exists(newName); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("profile %q already exists", newName)
	}

	oldDir, err := Dir(oldName)
	if err != nil {
		return err
	}

	newDir, err := Dir(newName)
	if err != nil {
		return err
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		return fmt.Errorf("failed renaming profile directory: %w", err)
	}

	if active, err := Active(); err == nil && active == oldName {
		return SetActive(newName)
	}

	return nil
}

// Active returns the name of the profile selected by `fly profile use`, or
// Default when none has been selected.
func Active() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}

	b, err := os.ReadFile(filepath.Join(home, activeFileName))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return Default, nil
	case err != nil:
		return "", err
	}

	name := strings.TrimSpace(string(b))
	if name == "" {
		return Default, nil
	}

	return name, nil
}

// SetActive records name as the active profile. Selecting the default profile
// clears the pointer rather than writing it, so an untouched installation
// leaves no trace.
func SetActive(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}

	home, err := Home()
	if err != nil {
		return err
	}

	path := filepath.Join(home, activeFileName)

	if name == Default {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		return nil
	}

	if err := os.MkdirAll(home, dirPerm); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(name+"\n"), filePerm)
}

// FindProjectFile walks up from dir looking for a ProjectFileName. It returns
// the path of the file and the profile name it names. Both are empty when no
// such file is found.
func FindProjectFile(dir string) (path, name string, err error) {
	if dir == "" {
		return "", "", nil
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}

	for {
		candidate := filepath.Join(dir, ProjectFileName)

		b, err := os.ReadFile(candidate)
		switch {
		case err == nil:
			name := strings.TrimSpace(string(b))
			if name == "" {
				return "", "", fmt.Errorf("%s is empty", candidate)
			}

			// Tolerate a trailing comment line so the file can explain itself.
			name, _, _ = strings.Cut(name, "\n")

			return candidate, strings.TrimSpace(name), nil
		case !errors.Is(err, os.ErrNotExist):
			return "", "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", nil
		}
		dir = parent
	}
}

// WriteProjectFile binds dir to the named profile.
func WriteProjectFile(dir, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}

	path := filepath.Join(dir, ProjectFileName)
	if err := os.WriteFile(path, []byte(name+"\n"), 0o644); err != nil {
		return "", err
	}

	return path, nil
}

// ResolveOptions carries the inputs to Resolve that cannot be read from the
// environment.
type ResolveOptions struct {
	// Flag is the value of the global --profile flag, empty when unset.
	Flag string

	// WorkingDir is where the search for a .fly-profile file starts. An empty
	// value skips that step.
	WorkingDir string
}

// Resolve decides which config directory flyctl should use, in descending
// order of precedence:
//
//  1. FLY_CONFIG_DIR, which pins a directory and bypasses profiles entirely
//  2. the --profile flag
//  3. the FLY_PROFILE environment variable
//  4. the nearest .fly-profile file at or above the working directory
//  5. the profile selected by `fly profile use`
//  6. the default profile
//
// A named profile that does not exist is an error rather than a silent
// fallback: quietly deploying to the wrong account is far worse than failing.
func Resolve(opts ResolveOptions) (Resolution, error) {
	if v := strings.TrimSpace(os.Getenv(ConfigDirEnvKey)); v != "" {
		return Resolution{Dir: v, Source: SourceConfigDirEnv}, nil
	}

	if name := strings.TrimSpace(opts.Flag); name != "" {
		return resolveNamed(name, SourceFlag, "")
	}

	if name := strings.TrimSpace(os.Getenv(EnvKey)); name != "" {
		return resolveNamed(name, SourceEnv, "")
	}

	path, name, err := FindProjectFile(opts.WorkingDir)
	if err != nil {
		return Resolution{}, err
	}
	if name != "" {
		return resolveNamed(name, SourceProjectFile, path)
	}

	active, err := Active()
	if err != nil {
		return Resolution{}, err
	}
	if active != Default {
		return resolveNamed(active, SourceActive, "")
	}

	dir, err := Dir(Default)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{Name: Default, Dir: dir, Source: SourceDefault}, nil
}

// TolerateUnresolvedAnnotation marks commands that must keep working when
// resolution fails.
//
// A .fly-profile file or an active pointer naming a deleted profile would
// otherwise lock the user out of the very commands that repair it, so the
// profile management commands fall back to the default profile and report the
// problem instead of refusing to run.
const TolerateUnresolvedAnnotation = "profile/tolerate-unresolved"

// Fallback returns a Resolution pointing at the default profile and carrying
// the error that prevented proper resolution.
func Fallback(cause error) (Resolution, error) {
	dir, err := Dir(Default)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{Name: Default, Dir: dir, Source: SourceDefault, Err: cause}, nil
}

// FlagFromArgs scrapes the value of the global profile flag out of a raw
// argument list.
//
// It exists because some config-directory consumers are initialized while the
// root command is being built, which is before cobra has parsed anything. Both
// `--profile name` and `--profile=name` are recognized.
func FlagFromArgs(args []string) string {
	const long = "--" + FlagName

	for i, arg := range args {
		switch {
		case arg == long:
			if i+1 < len(args) {
				return strings.TrimSpace(args[i+1])
			}
		case strings.HasPrefix(arg, long+"="):
			return strings.TrimSpace(strings.TrimPrefix(arg, long+"="))
		}
	}

	return ""
}

func resolveNamed(name string, source Source, detail string) (Resolution, error) {
	where := string(source)
	if detail != "" {
		where = detail
	}

	if err := ValidateName(name); err != nil {
		return Resolution{}, fmt.Errorf("profile selected by %s is invalid: %w", where, err)
	}

	exists, err := Exists(name)
	if err != nil {
		return Resolution{}, err
	}
	if !exists {
		return Resolution{}, fmt.Errorf(
			"profile %q (selected by %s) does not exist; create it with `fly profile add %s`",
			name, where, name,
		)
	}

	dir, err := Dir(name)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{Name: name, Dir: dir, Source: source, Detail: detail}, nil
}

// ReadMetadata returns the router metadata stored for the named profile.
func ReadMetadata(name string) (Metadata, error) {
	dir, err := Dir(name)
	if err != nil {
		return Metadata{}, err
	}

	return readMetadata(dir), nil
}

// WriteMetadata stores router metadata for the named profile.
func WriteMetadata(name string, md Metadata) error {
	dir, err := Dir(name)
	if err != nil {
		return err
	}

	b, err := yaml.Marshal(md)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, MetadataFileName), b, filePerm)
}

// readMetadata is best-effort: metadata is a display convenience, so a
// missing or corrupt file yields a zero value rather than an error.
func readMetadata(dir string) (md Metadata) {
	b, err := os.ReadFile(filepath.Join(dir, MetadataFileName))
	if err != nil {
		return
	}

	_ = yaml.Unmarshal(b, &md)

	return
}
