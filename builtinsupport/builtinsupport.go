package builtinsupport

import (
	"fmt"
	"strings"
	"text/template"
)

// Arg is a simple holder for names and defaults in args
type Arg struct {
	Name    string
	Default string
}

// Builtin - Definition of a Fly Builtin Builder
type Builtin struct {
	Name        string
	Description string
	Details     string
	FileText    string
	BuiltinArgs []Arg
}

var builtins map[string]Builtin

// GetBuiltin - Finds the Builtin by name
func GetBuiltin(builtinname string) (*Builtin, error) {
	initBuiltins()

	builtin, ok := builtins[builtinname]

	if !ok {
		return nil, fmt.Errorf("no builtin with %s name supported", builtinname)
	}

	return &builtin, nil
}

// GetVDockerfile - given an map of variables, get the definition and populate it
func (b *Builtin) GetVDockerfile(vars map[string]string) (string, error) {
	template, err := template.New("builtin").Parse(b.FileText)

	if err != nil {
		return "", err
	}

	// Now the create the proper vars from
	// If it's set in the vars map, set it in the settings map

	settings := make(map[string]string, len(vars))

	if vars != nil {
		for k, v := range vars {
			if b.BuiltinArgs != nil {
				for _, arg := range b.BuiltinArgs {
					if arg.Name == k {
						// This is good to add
						settings[k] = v
						break
					}
				}
			}
		}
	}

	// settings now has all the values which were in Builtinargs, but no others

	// Now scan builtinargs for any value not set and copy the default over
	for _, arg := range b.BuiltinArgs {
		_, found := settings[arg.Name]
		if !found {
			// This is good to set to default
			settings[arg.Name] = arg.Default
			break
		}
	}

	result := strings.Builder{}

	err = template.Execute(&result, settings)
	if err != nil {
		return "", err
	}

	return result.String(), nil
}

// GetBuiltins - Get an array of all the builtins
func GetBuiltins() []Builtin {
	initBuiltins()

	var builtarray []Builtin

	for _, v := range builtins {
		builtarray = append(builtarray, v)
	}

	return builtarray
}

// Internal function to load up builtins
func initBuiltins() {
	if len(builtins) != 0 {
		return
	}
	builtins = make(map[string]Builtin)

	for _, rt := range basicbuiltins {
		builtins[rt.Name] = rt
	}
}
