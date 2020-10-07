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
		builtin.BuiltinArgs = []Arg{}
		if tree.HasPath([]string{v, "args"}) {

			argp := tree.GetPath([]string{v, "args"}).(*toml.Tree)
			args := argp.Keys()

			for _, k := range args {
				var myarg Arg
				myarg.Name = k
				argmap := tree.GetPath([]string{v, "args", k}).(*toml.Tree).ToMap()
				myarg.Default = argmap["default"]
				myarg.Description = argmap["description"].(string)
				builtin.BuiltinArgs = append(builtin.BuiltinArgs, myarg)
			}

		}
		builtins = append(builtins, builtin)
	}
	return builtins, nil
}
