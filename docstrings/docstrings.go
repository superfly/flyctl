package docstrings

//go:generate ruby ../scripts/generate_cmd_docs.rb ../cmddocs.yaml ./gen.go

type KeyStrings struct {
	Usage string
	Short string
	Long  string
}
