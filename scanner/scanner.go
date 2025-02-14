package scanner

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/iostreams"
)

//go:embed templates templates/*/.dockerignore templates/**/.fly templates/**/.github
var content embed.FS

type InitCommand struct {
	Command     string
	Args        []string
	Description string
	Condition   bool
}

type Secret struct {
	Key      string
	Help     string
	Value    string
	Generate func() (string, error)
}

type MergeConfigStruct struct {
	Name      string
	Temporary bool
}

type DatabaseKind int

const (
	DatabaseKindNone DatabaseKind = iota
	DatabaseKindPostgres
	DatabaseKindMySQL
	DatabaseKindSqlite
)

type SourceInfo struct {
	Family           string
	Version          string
	DockerfilePath   string
	BuildArgs        map[string]string
	Builder          string
	ReleaseCmd       string
	SeedCmd          string
	DockerCommand    string
	DockerEntrypoint string
	KillSignal       string
	SwapSizeMB       int
	Buildpacks       []string
	Secrets          []Secret

	Files                           []SourceFile
	Port                            int
	Env                             map[string]string
	Statics                         []Static
	Processes                       map[string]string
	DeployDocs                      string
	Notice                          string
	SkipDeploy                      bool
	SkipDatabase                    bool
	Volumes                         []Volume
	DockerfileAppendix              []string
	InitCommands                    []InitCommand
	PostgresInitCommands            []InitCommand
	PostgresInitCommandCondition    bool
	DatabaseDesired                 DatabaseKind
	RedisDesired                    bool
	GitHubActions                   GitHubActionsStruct
	ObjectStorageDesired            bool
	OverrideExtensionSecretKeyNames map[string]map[string]string
	Concurrency                     map[string]int
	Callback                        func(appName string, srcInfo *SourceInfo, plan *plan.LaunchPlan, flags []string) error
	HttpCheckPath                   string
	HttpCheckHeaders                map[string]string
	ConsoleCommand                  string
	MergeConfig                     *MergeConfigStruct
	AutoInstrumentErrors            bool
	FailureCallback                 func(err error) error
	Runtime                         plan.RuntimeStruct
	PostInitCallback                func() error
}

type SourceFile struct {
	Path     string
	Contents []byte
}

type Static = appconfig.Static

type Volume = appconfig.Mount

type ScannerConfig struct {
	Mode         string
	ExistingPort int
	Colorize     *iostreams.ColorScheme
}

type GitHubActionsStruct struct {
	Deploy  bool
	Secrets bool
	Files   []SourceFile
}

func Scan(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	scanners := []sourceScanner{
		configureDjango,
		configureLaravel,
		configurePhoenix,
		configureRails,
		configureRedwood,
		configureJsFramework,
		/* frameworks scanners are placed before generic scanners,
		   since they might mix languages or have a Dockerfile that
			 doesn't work with Fly */
		configureDockerfile,
		configureBridgetown,
		configureLucky,
		configureRuby,
		configureGo,
		configureElixir,
		configureFlask,
		configurePython,
		configureDeno,
		configureNuxt,
		configureNextJs,
		configureNode,
		configureStatic,
		configureDotnet,
		configureRust,
	}

	for _, scanner := range scanners {
		si, err := scanner(sourceDir, config)
		if err != nil {
			return nil, err
		}
		if si != nil {
			github_actions(sourceDir, &si.GitHubActions)
			return si, nil
		}
	}

	return nil, nil
}

type sourceScanner func(sourceDir string, config *ScannerConfig) (*SourceInfo, error)

// templates recursively returns files from the templates directory within the named directory
// will panic on errors since these files are embedded and should work
func templates(name string) (files []SourceFile) {
	filter := func(input []byte) []byte { return input }
	return templatesFilter(name, filter)
}

// same thing as templates (above) but with template execution given a map of variables
func templatesExecute(name string, vars map[string]interface{}) (files []SourceFile) {
	filter := func(input []byte) []byte {
		template := template.Must(template.New("name").Parse(string(input)))
		result := strings.Builder{}
		err := template.Execute(&result, vars)
		if err != nil {
			panic(err)
		}

		return []byte(result.String())
	}

	return templatesFilter(name, filter)
}

// templates with a filter function applied to the content of each template
func templatesFilter(name string, filter func(input []byte) []byte) (files []SourceFile) {
	err := fs.WalkDir(content, name, func(path string, d fs.DirEntry, e error) error {
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(name, path)
		if err != nil {
			return errors.Wrap(err, "error removing template prefix")
		}

		data, err := fs.ReadFile(content, path)
		if err != nil {
			return err
		}

		f := SourceFile{
			Path:     relPath,
			Contents: filter(data),
		}

		files = append(files, f)
		return nil
	})
	if err != nil {
		panic(err)
	}

	return
}
