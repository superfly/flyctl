package docstrings

//go:generate make -C .. cmddocs

type KeyStrings struct {
	Usage string
	Short string
	Long  string
}
