package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pelletier/go-toml"
)

func main() {
	readFile := os.Args[1]

	tree, err := toml.LoadFile(readFile)

	if err != nil {
		log.Fatal("Can't parse docStrings")
	}

	mapped := tree.ToMap()

	fmt.Println("package docstrings\n\n// Get - Get a document string\nfunc Get(key string) KeyStrings {switch(key) {")

	dumpMap("", mapped)

	fmt.Println("}\npanic(\"unknown command key \" + key)\n}")
}

func dumpMap(prefix string, m map[string]interface{}) {
	_, prs := m["usage"]
	if prs {
		usage := m["usage"].(string)
		short := m["shortHelp"].(string)
		long := m["longHelp"].(string)
		fmt.Printf("case \"%s\":\nreturn KeyStrings{\"%s\",\"%s\",\n    `%s`,\n}\n",
			prefix, strings.TrimSpace(usage), strings.TrimSpace(short), strings.TrimSpace(long))
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
			fmt.Fprintln(os.Stderr, "Node ", node, " not handled")
		}
	}
}
