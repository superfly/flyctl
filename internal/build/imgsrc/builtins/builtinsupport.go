package builtins

import (
	"fmt"
	"strings"
	"text/template"
)

// Setting is a simple holder for names and defaults in Settings
type Setting struct {
	Name        string
	Default     interface{}
	Description string
}

// Builtin - Definition of a Fly Builtin Builder
type Builtin struct {
	Name        string
	Description string
	Details     string
	Template    string
	Settings    []Setting
	settingsMap map[string]Setting
}

// GetBuiltin - Finds the Builtin by name
func GetBuiltin(builtinname string) (*Builtin, error) {
	for _, builtin := range basicbuiltins {
		if builtin.Name == builtinname {
			return &builtin, nil
		}
	}

	return nil, fmt.Errorf("no builtin with %s name supported", builtinname)
}

// ResolveSettings - Given defaults abd values return actural settings
func (b *Builtin) ResolveSettings(vars map[string]interface{}) map[string]interface{} {
	resolvedSettings := make(map[string]interface{}, len(vars))

	for k, v := range vars {
		if b.Settings != nil {
			for _, setting := range b.Settings {
				if setting.Name == k {
					// This is good to add
					resolvedSettings[k] = v
					break
				}
			}
		}
	}

	// settings now has all the values which were in settings, but no others

	// Now scan settings for any value not set and copy the default over
	for _, setting := range b.Settings {
		_, found := resolvedSettings[setting.Name]
		if !found {
			// This is good to set to default
			resolvedSettings[setting.Name] = setting.Default
		}
	}

	return resolvedSettings
}

// GetVDockerfile - given an map of variables, get the definition and populate it
func (b *Builtin) GetVDockerfile(vars map[string]interface{}) (string, error) {
	template, err := template.New("builtin").Parse(b.Template)

	if err != nil {
		return "", err
	}

	// Now the create the proper vars from
	// If it's set in the vars map, set it in the settings map

	settings := b.ResolveSettings(vars)

	result := strings.Builder{}

	err = template.Execute(&result, settings)
	if err != nil {
		return "", err
	}

	return result.String(), nil
}

// GetSetting - Gets the Setting structure for a named setting
func (b *Builtin) GetSetting(name string) Setting {
	if len(b.settingsMap) != len(b.Settings) {
		b.settingsMap = make(map[string]Setting)
		for _, a := range b.Settings {
			b.settingsMap[a.Name] = a
		}
	}

	return b.settingsMap[name]
}

// GetBuiltins - Get an array of all the builtins
func GetBuiltins() []Builtin {
	return basicbuiltins
}
