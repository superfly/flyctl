package main

import (
	"fmt"
	"github.com/pelletier/go-toml"
	"log"
	"os"
)

func main() {
	readFile := os.Args[1]

	tree, err := toml.LoadFile(readFile)

	if err != nil {
		log.Fatal("Can't parse docStrings")
	}

	mapped := tree.ToMap()

	fmt.Println("package docstrings\n\nvar docstrings=map[string]KeyStrings{")

	dumpMap("", mapped)

	fmt.Println("}")
}

func dumpMap(prefix string, m map[string]interface{}) {
	_, prs := m["usage"]
	if prs {
		usage := m["usage"].(string)
		short := m["shortHelp"].(string)
		long := m["longHelp"].(string)
		fmt.Printf("    \"%s\":KeyStrings{\"%s\",\"%s\",\n    `%s`,\n},\n", prefix, usage, short, long)
	}

	for k, v := range m {
		switch node := v.(type) {
		case map[string]interface{}:
			if prefix != "" {
				dumpMap(prefix+"."+k, v.(map[string]interface{}))
			} else {
				dumpMap(k, v.(map[string]interface{}))
			}
		case string:
			// Nothing to do
		default:
			fmt.Println("Node ", node, " not handled")
		}
	}
}
