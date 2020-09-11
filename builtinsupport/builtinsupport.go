package builtinsupport

import (
	"fmt"
	"strings"
	"text/template"
)

// BuiltinArg is a simple holder for names and defaults in args
type BuiltinArg struct {
	Name    string
	Default string
}

type Builtin struct {
	Name        string
	Description string
	Details     string
	FileText    string
	Args        []BuiltinArg
}

var builtins map[string]Builtin

func GetBuiltin(builtinname string) (*Builtin, error) {
	initBuiltins()

	builtin, ok := builtins[builtinname]

	if !ok {
		return nil, fmt.Errorf("no builtin with %s name supported", builtinname)
	}

	return &builtin, nil
}

func (b *Builtin) GetVDockerfile(vars map[string]string) (string, error) {
	template, err := template.New("builtin").Parse(b.FileText)

	if err != nil {
		return "", err
	}
	result := strings.Builder{}

	err = template.Execute(&result, vars)
	if err != nil {
		return "", err
	}
	return result.String(), nil
}

func GetBuiltins() []Builtin {
	initBuiltins()

	var builtarray []Builtin

	for _, v := range builtins {
		builtarray = append(builtarray, v)
	}

	return builtarray
}

func initBuiltins() {
	if len(builtins) != 0 {
		return
	}
	builtins = make(map[string]Builtin)

	for _, rt := range basicbuiltins {
		builtins[rt.Name] = rt
	}
}
