package builtinsupport

import (
	"github.com/pelletier/go-toml"
)

func loadBuiltins(filename string) (br []Builtin, err error) {

	tree, err := toml.LoadFile(filename)
	if err != nil {
		return nil, err
	}

	keys := tree.Keys()
	var builtins []Builtin

	for _, v := range keys {

		var builtin Builtin

		builtin.Name = v
		builtin.Description = tree.GetPath([]string{v, "description"}).(string)
		builtin.Details = tree.GetPath([]string{v, "details"}).(string)
		builtin.Template = tree.GetPath([]string{v, "template"}).(string)
		builtin.Settings = []Setting{}
		if tree.HasPath([]string{v, "settings"}) {

			settingp := tree.GetPath([]string{v, "settings"}).(*toml.Tree)
			settings := settingp.Keys()

			for _, k := range settings {
				var mySetting Setting
				mySetting.Name = k
				settingmap := tree.GetPath([]string{v, "settings", k}).(*toml.Tree).ToMap()
				mySetting.Default = settingmap["default"]
				mySetting.Description = settingmap["description"].(string)
				builtin.Settings = append(builtin.Settings, mySetting)
			}

		}
		builtins = append(builtins, builtin)
	}
	return builtins, nil
}
