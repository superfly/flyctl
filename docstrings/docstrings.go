package docstrings

import (
	"github.com/pelletier/go-toml"
	"log"
)

type KeyStrings struct {
	Usage string
	Short string
	Long  string
}

type DocStrings struct {
	tree *toml.Tree
}

var docStrings *DocStrings

func load() {
	var err error

	docStrings = &DocStrings{}

	docStrings.tree, err = toml.Load(Flyctldocstrings)

	if err != nil {
		log.Fatal("Can't parse docStrings")
	}

	return
}

func Get(docKey string) (k KeyStrings) {
	if docStrings == nil {
		load()
	}

	if !docStrings.tree.Has(docKey) {
		log.Fatal("Doc key missing for ", docKey)
	}
	gotStrings := docStrings.tree.Get(docKey).(*toml.Tree)

	info := gotStrings.Get("usage").(string)
	long := gotStrings.Get("longHelp").(string)
	short := gotStrings.Get("shortHelp").(string)

	return KeyStrings{info, short, long}
}
