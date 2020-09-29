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

		builtin.Description = tree.GetPath([]string{v, "Description"}).(string)
		builtin.Details = tree.GetPath([]string{v, "Details"}).(string)
		builtin.Template = tree.GetPath([]string{v, "Template"}).(string)
		builtin.BuiltinArgs = []Arg{}
		if tree.HasPath([]string{v, "Args"}) {

			argp := tree.GetPath([]string{v, "Args"}).(*toml.Tree)
			args := argp.ToMap()

			for k, v := range args {
				builtin.BuiltinArgs = append(builtin.BuiltinArgs, Arg{k, v})
			}

		}
		builtins = append(builtins, builtin)
	}
	return builtins, nil
}
