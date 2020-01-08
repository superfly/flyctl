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

	fmt.Println("package docstrings\n\nvar docstrings=map[string]KeyStrings{")
	for _, k := range tree.Keys() {
		v := tree.Get(k).(*toml.Tree)
		usage := v.Get("usage").(string)
		short := v.Get("shortHelp").(string)
		long := v.Get("longHelp").(string)
		fmt.Printf("    \"%s\":KeyStrings{\"%s\",\"%s\",\n    `%s`,\n},\n", k, usage, short, long)
	}

	fmt.Println("}")
}
