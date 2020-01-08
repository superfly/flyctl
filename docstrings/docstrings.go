package docstrings

type KeyStrings struct {
	Usage string
	Short string
	Long  string
}

func Get(docKey string) (k KeyStrings) {
	return docstrings[docKey]
}
