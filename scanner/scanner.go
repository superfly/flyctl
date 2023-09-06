package scanner

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/command/launch/plan"
)

//go:embed templates templates/*/.dockerignore templates/**/.fly
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
	Family                       string
	Version                      string
	DockerfilePath               string
	BuildArgs                    map[string]string
	Builder                      string
	ReleaseCmd                   string
	DockerCommand                string
	DockerEntrypoint             string
	KillSignal                   string
	SwapSizeMB                   int
	Buildpacks                   []string
	Secrets                      []Secret
	Files                        []SourceFile
	Port                         int
	Env                          map[string]string
	Statics                      []Static
	Processes                    map[string]string
	DeployDocs                   string
	Notice                       string
	SkipDeploy                   bool
	SkipDatabase                 bool
	Volumes                      []Volume
	DockerfileAppendix           []string
	InitCommands                 []InitCommand
	PostgresInitCommands         []InitCommand
	PostgresInitCommandCondition bool
	DatabaseDesired              DatabaseKind
	RedisDesired                 bool
	Concurrency                  map[string]int
	Callback                     func(appName string, srcInfo *SourceInfo, plan *plan.LaunchPlan) error
	HttpCheckPath                string
	HttpCheckHeaders             map[string]string
	ConsoleCommand               string
	MergeConfig                  *MergeConfigStruct
	AutoInstrumentErrors         bool
}

type SourceFile struct {
	Path     string
	Contents []byte
}
type Static struct {
	GuestPath string `toml:"guest_path" json:"guest_path"`
	UrlPrefix string `toml:"url_prefix" json:"url_prefix"`
}
type Volume struct {
	Source      string   `toml:"source" json:"source,omitempty"`
	Destination string   `toml:"destination" json:"destination,omitempty"`
	Processes   []string `json:"processes,omitempty" toml:"processes,omitempty"`
}
type ScannerConfig struct {
	Mode         string
	ExistingPort int
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
		configureLucky,
		configureRuby,
		configureGo,
		configureElixir,
		configurePython,
		configureDeno,
		configureNuxt,
		configureNextJs,
		configureNode,
		configureStatic,
		configureDotnet,
	}

	for _, scanner := range scanners {
		si, err := scanner(sourceDir, config)
		if err != nil {
			return nil, err
		}
		if si != nil {
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
